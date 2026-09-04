package hub

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/stranix79/deckhand/internal/deck"
	"github.com/stranix79/deckhand/internal/session"
)

// DeckRow is a decks row.
type DeckRow struct {
	ID, UserID, Slug, Title string
	Version                 int
	Ratio                   string
	Width, SlideCount       int
	SizeBytes               int64
	Path                    string
	PermalinkCode           string
	CreatedAt, UpdatedAt    time.Time
	ExpiresAt               *time.Time
}

// Expired is true when the free-plan permanent link has lapsed.
func (d *DeckRow) Expired() bool { return d.ExpiresAt != nil && d.ExpiresAt.Before(time.Now()) }

// ErrPlanLimit is returned when the free plan does not allow the action.
var ErrPlanLimit = errors.New("free plan limit reached")

var slugClean = regexp.MustCompile(`[^a-z0-9]+`)

// slugify makes a URL slug: "Ship it before lunch!" → "ship-it-before-lunch".
func slugify(s string) string {
	s = strings.Trim(slugClean.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 48 {
		s = strings.Trim(s[:48], "-")
	}
	if s == "" {
		s = "deck"
	}
	return s
}

const deckCols = `id, user_id, slug, title, version, ratio, width, slide_count, size_bytes, path, COALESCE(permalink_code, ''), created_at, updated_at, expires_at`

func scanDeck(row pgx.Row) (*DeckRow, error) {
	var d DeckRow
	err := row.Scan(&d.ID, &d.UserID, &d.Slug, &d.Title, &d.Version, &d.Ratio, &d.Width, &d.SlideCount, &d.SizeBytes, &d.Path, &d.PermalinkCode, &d.CreatedAt, &d.UpdatedAt, &d.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

// storeDeck validates an uploaded archive and stores it as a new version of
// the user's deck with that slug (or a new deck). The archive is read at
// most MaxDeckBytes; the extracted deck obeys the usual limits.
func (h *Hub) storeDeck(ctx context.Context, u *User, src io.Reader, filename, slugHint string) (*DeckRow, error) {
	ext := archiveExt(filename)
	if ext == "" {
		return nil, &deck.Report{Errors: []string{"upload a .zip or a .tar.gz of your deck"}}
	}
	tmp, err := os.CreateTemp("", "deckhand-upload-*"+ext)
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	n, err := io.Copy(tmp, io.LimitReader(src, h.cfg.MaxDeckBytes+1))
	_ = tmp.Close()
	if err != nil {
		return nil, err
	}
	if n > h.cfg.MaxDeckBytes {
		return nil, &deck.Report{Errors: []string{fmt.Sprintf("archive larger than %d MB", h.cfg.MaxDeckBytes>>20)}}
	}
	limits := deck.DefaultLimits
	limits.MaxBytes = h.cfg.MaxDeckBytes
	d, err := deck.Load(tmp.Name(), deck.WithLimits(limits))
	if err != nil {
		return nil, err
	}
	defer func() { _ = d.Close() }()

	slug := slugify(slugHint)
	if slugHint == "" {
		slug = slugify(d.Title)
	}

	// Free plan: one deck. Updating an existing slug is always allowed.
	var exists bool
	if err := h.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM decks WHERE user_id = $1 AND slug = $2)`, u.ID, slug).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists && !u.Paid {
		var count int
		if err := h.db.QueryRow(ctx, `SELECT count(*) FROM decks WHERE user_id = $1`, u.ID).Scan(&count); err != nil {
			return nil, err
		}
		if count >= h.cfg.FreeMaxDecks {
			return nil, ErrPlanLimit
		}
	}

	var expires *time.Time
	if !u.Paid {
		t := time.Now().Add(time.Duration(h.cfg.FreeLinkDays) * 24 * time.Hour)
		expires = &t
	}
	var id string
	var version int
	var oldPath string
	err = h.db.QueryRow(ctx, `
		INSERT INTO decks (user_id, slug, title, ratio, width, slide_count, size_bytes, path, permalink_code, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, '', $8, $9)
		ON CONFLICT (user_id, slug) DO UPDATE SET
		  version = decks.version + 1, title = EXCLUDED.title, ratio = EXCLUDED.ratio, width = EXCLUDED.width,
		  slide_count = EXCLUDED.slide_count, size_bytes = EXCLUDED.size_bytes, updated_at = now(), expires_at = EXCLUDED.expires_at
		RETURNING id, version, path`,
		u.ID, slug, d.Title, d.Ratio, d.Width, len(d.Slides), n, session.NewCode(), expires).Scan(&id, &version, &oldPath)
	if err != nil {
		return nil, err
	}
	dest := filepath.Join(h.cfg.DataDir, "decks", id, fmt.Sprintf("v%d", version))
	if err := copyDir(d.Root, dest); err != nil {
		return nil, err
	}
	if _, err := h.db.Exec(ctx, `UPDATE decks SET path = $1 WHERE id = $2`, dest, id); err != nil {
		return nil, err
	}
	if oldPath != "" && oldPath != dest {
		_ = os.RemoveAll(oldPath)
	}
	// A new version replaces the deck of live sessions on their next load;
	// running presentations keep the version they started with (their
	// session holds the old Root until it ends). The permalink session is
	// refreshed now so /d/… shows the new version.
	row, err := h.getDeck(ctx, id)
	if err != nil {
		return nil, err
	}
	if row.PermalinkCode != "" {
		h.sessions.Remove(row.PermalinkCode)
	}
	return row, nil
}

func archiveExt(name string) string {
	l := strings.ToLower(name)
	switch {
	case strings.HasSuffix(l, ".zip"):
		return ".zip"
	case strings.HasSuffix(l, ".tar.gz"):
		return ".tar.gz"
	case strings.HasSuffix(l, ".tgz"):
		return ".tgz"
	}
	return ""
}

// copyDir copies a validated deck tree (regular files only). The source is
// our own temporary extraction, which contains no symlinks (refused at load).
func copyDir(src, dst string) error { //nolint:gosec // see above
	return filepath.WalkDir(src, func(p string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if e.IsDir() {
			return os.MkdirAll(target, 0o750)
		}
		if !e.Type().IsRegular() {
			return nil
		}
		in, err := os.Open(p) //nolint:gosec // validated tree
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	})
}

func (h *Hub) getDeck(ctx context.Context, id string) (*DeckRow, error) {
	return scanDeck(h.db.QueryRow(ctx, `SELECT `+deckCols+` FROM decks WHERE id = $1`, id))
}

func (h *Hub) getDeckByHandleSlug(ctx context.Context, handle, slug string) (*DeckRow, error) {
	return scanDeck(h.db.QueryRow(ctx, `SELECT `+deckCols+` FROM decks d WHERE d.slug = $2 AND d.user_id = (SELECT id FROM users WHERE handle = $1)`, handle, slug))
}

func (h *Hub) getDeckBySlug(ctx context.Context, userID, slug string) (*DeckRow, error) {
	return scanDeck(h.db.QueryRow(ctx, `SELECT `+deckCols+` FROM decks WHERE user_id = $1 AND slug = $2`, userID, slug))
}

func (h *Hub) listDecks(ctx context.Context, userID string) ([]*DeckRow, error) {
	rows, err := h.db.Query(ctx, `SELECT `+deckCols+` FROM decks WHERE user_id = $1 ORDER BY updated_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*DeckRow
	for rows.Next() {
		d, err := scanDeck(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// deleteDeck removes a deck, its files and ends its presentations.
func (h *Hub) deleteDeck(ctx context.Context, u *User, id string) error {
	d, err := h.getDeck(ctx, id)
	if err != nil || d.UserID != u.ID {
		return pgx.ErrNoRows
	}
	rows, err := h.db.Query(ctx, `SELECT code FROM presentations WHERE deck_id = $1 AND ended_at IS NULL`, id)
	if err != nil {
		return err
	}
	var codes []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err == nil {
			codes = append(codes, c)
		}
	}
	rows.Close()
	for _, c := range codes {
		h.sessions.Remove(c)
	}
	if _, err := h.db.Exec(ctx, `DELETE FROM decks WHERE id = $1`, id); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(h.cfg.DataDir, "decks", id)) //nolint:gosec // id is a UUID from the database
}

// deckURL is the permanent link of a deck.
func (h *Hub) deckURL(handle, slug string) string {
	return h.cfg.BaseURL + "/d/" + handle + "/" + slug
}
