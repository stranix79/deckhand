# Deckhand vs Google Slides

Google Slides is a browser-based slide editor with real-time collaboration,
templates, comments, version history and Gemini built in; it runs on
Google's servers and your decks live in Drive. Deckhand is a single binary
that presents a folder of HTML files from your machine. They do not compete
on authoring at all: Slides is an editor, Deckhand is not. The overlap is
the moment you present: notes on a phone, a remote, the audience on their
own devices.

## Owning the deck

A Google Slides deck is a document in Google's format. You can export it to
PPTX, PDF, ODP, images or plain text, and every export loses something. You
need a Google account to make one; viewers of a shared link do not.

A Deckhand deck is a folder: `01-title.html`, `02-problem.html`, an optional
`deck.json` with notes. Open any file in a browser and it is your slide. Put
the folder in git and keep it for twenty years. The trade-off is plain:
there is no editor. You write the HTML, generate it from data, or ask a
language model to produce it.

## Presenting

Google Slides Presenter View shows notes, a timer and the next slide in a
second window on the presenting computer. From the phone app you can present
on the phone itself, cast to a Chromecast (with the notes on the phone) or
present into a Google Meet. Using the phone as a remote for a presentation
running on a laptop is not a built-in feature; people use third-party
extensions for that.

Audience tools give you Q&A: a short link appears at the top of the slides,
people open it from any device and post questions, vote on them, and you
can put a question on screen. In Workspace you can restrict who may ask.
That link is for questions; the audience does not see the slides through
it. To follow along they need the deck shared with them, and their copy
does not move with you.

Deckhand prints a remote link and an audience link with QR codes. The phone
becomes the remote, with current and next slide, notes, timer, laser
pointer, black screen and a way to show the audience QR on the stage.
Everyone who scans the audience QR follows the slides live on their own
screen, over the LAN, with no account and no app, and can browse back and
forth on their own. There is no Q&A feature. Everything runs on your
machine; the optional Hub adds viewers outside the room, permanent links
and audience statistics.

## Side by side

| | Deckhand | Google Slides |
|---|---|---|
| Editing | None; HTML by hand, script or AI | Full browser editor, templates, Gemini |
| Collaboration | Git, if you want it | Real-time co-editing, comments, history |
| File format | HTML files you own | Google's format; export to PPTX, PDF, ODP, images |
| Account needed | No (Hub: e-mail magic link, optional) | Yes to create; no to view a public link |
| Works offline | Yes, entirely | Partly, with Chrome offline access |
| Presenter notes and timer | On your phone | Presenter View window on the computer |
| Phone as remote for the big screen | Yes, built in, QR code | Not built in (third-party extensions) |
| Present from the phone itself | No | Yes: on device, Chromecast, Meet |
| Audience follows live on their device | Yes, LAN link, synced, no account | No; the Q&A link only |
| Audience Q&A | No | Yes, with voting and moderation |
| Slide isolation | Sandboxed iframe, strict CSP | Not applicable |
| Install | One binary | Nothing, a browser |
| Cost | Free and open source; Hub free tier, Pro plan | Free with a Google account; Workspace plans |

## When to pick Google Slides instead

Pick Google Slides when several people build the deck together, when the
deck has to look fine without anyone writing CSS, when your organisation
already lives in Workspace, when you need Q&A from the audience, or when you
will present through Meet. Deckhand has nothing to offer on any of those points.

## When Deckhand fits

Pick Deckhand when you want to own the slides as files, when the room may
have no internet, when you want the audience on their own screens without
accounts, or when the deck is produced by an AI or a script and you refuse
to paste it into an editor one slide at a time. Pick it when the phone remote
has to work in any venue with nothing to install.

## Try it

`brew install stranix79/tap/deckhand`, then
`deckhand present examples/ship-it --open`. The format and the prompt for
generating a deck are in the
[README](https://github.com/stranix79/deckhand#readme); the hosted Hub is
at [deckhand.show](https://deckhand.show/).
