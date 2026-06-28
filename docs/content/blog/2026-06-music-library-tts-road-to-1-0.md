---
title: "AfterTouch: From Rescue to Something Better, and the Road to 1.0"
date: 2026-06-28
description: "Since v0.93.1, AfterTouch grew from a cloud-shutdown rescue into a platform of its own: local music, voice prompts, sturdier internals, a growing community, and a 1.0 on the horizon."
tags:
  - discovery
  - health
  - migration
  - fixes
sidebar:
  exclude: true
---

The launch post went out under the wire. Bose pulled the plug on the SoundTouch cloud on
May 6, and **v0.93.1** was very much a rescue: get accounts migrated, keep radio and
presets alive, stop perfectly good speakers from turning into bricks. The weeks since,
up through **v0.117.0**, have been about a quieter shift: turning that rescue into
something that stands on its own, and in a few places, something better than what Bose
offered. And almost none of that direction came from me. I use my own speakers with a
pretty narrow set of features; nearly everything below exists because someone in the
community described a use case I'd never have thought to build.

## Local music, back under your control, and a speaker that talks

The clearest sign of that shift is local music. Your speakers always had a native
local-music source for playing your own library off the network, but browsing it used to
run through the Bose app. AfterTouch brings that back on its own terms: it discovers
DLNA / UPnP media servers on your network and drives the speaker's native source
directly. Browse folders in the **Library** tab or from the command line, queue a whole
folder, and next/previous and auto-advance behave like a real playlist.

Then there's something genuinely new: speakers can now *talk*. A text-to-speech feature
announces arbitrary text out loud, with Google Cloud TTS as a pluggable provider you
configure from the settings UI. It's built on the speaker's notification capability, but
turning that into spoken prompts is the kind of thing that happens when the platform is
open and nobody has to wait for a vendor to approve it.

There is more in the same spirit, smaller but useful: service-side search across TuneIn
and Radio Browser, a "Play URL" view for arbitrary streams, save-as-preset straight from
Now Playing, and a step toward needing no extra hardware at all, an on-device SSH unlock
flow that opens the door to running AfterTouch directly on the speaker.

## The unglamorous half: earning trust

Features are the easy part to write about. The work that actually mattered most was
making AfterTouch dependable enough that you stop thinking about it. Speaker data is now
written to disk durably, so a power cut mid-write no longer wipes your presets and
accounts, and corrupt or empty files fall back to sane defaults instead of failing.
Recent tracks stopped vanishing and duplicating. Internet radio got steadier: Radio
Browser plays through its proper native source, TuneIn fails over across stream
candidates, and a stray trailing slash in a server URL no longer breaks playback.
Multi-room grouping handles member removal correctly.

Under the surface, a sustained pass closed several request-forgery paths, swept the code
for log-injection, validated identifiers on management endpoints, and removed a
credential-logging shortcut. And the health checks grew teeth: server-URL reachability,
CA-bundle integrity, a speaker-clock check with a one-click fix, and a DNS-path probe for
the internet-radio escape problem, all now labelled with the device name and IP so you
know exactly which speaker a warning is about.

## A community, not a product

The best thing to happen since launch isn't in the changelog. It's the people.

It's worth saying plainly: this project is driven by its users. I personally use
SoundTouch in a fairly simple way, and most of what shipped over these weeks (features
and bug fixes alike) is the result of friendly, constructive feedback from people who use
their speakers very differently than I do. The DLNA library, the voice prompts, the radio
and grouping fixes, the migration edge cases: each one started as someone taking the time
to explain a real-world setup and point at what was missing. That feedback is the
roadmap. Keep it coming.

A standout is **[Sander ten Brinke](https://x.com/sandertenbrinke)**, who is building
**[soundtouch-maui](https://github.com/sander1095/soundtouch-maui)**, a cross-platform
SoundTouch app designed to work hand in hand with AfterTouch. That's exactly the shape
this project should take: not one tool trying to do everything, but independent pieces
that fit together because they share an open, community-owned foundation. Go build a
player, a remote, a home-automation bridge, whatever you need, and have it talk to a
service you control.

An honest admission: there has been more activity in issues and discussions than one
maintainer can keep up with, and not every thread got the reply it deserved. But the
encouraging part is that it increasingly doesn't have to. People are answering each
other, sharing setups (the FRITZ!Box and AdGuard DNS notes came straight from a user's
own working configuration), and debugging together. That's the project moving in the
right direction. AfterTouch works best as a community, not a support desk.

## The road to 1.0

So what does **v1.0.0** mean? Mostly: stability. A version number that signals a proper,
dependable base you can build on, with a management API that won't shift under you and a
service that runs unprivileged and installs cleanly by default.

A few things are on the list to get there. The admin and account-management UI works,
but it feels rough at the edges, and that's the part you actually touch, so it deserves
some polish. I also want to keep a publicly deployed, cloud-hosted service in mind:
the moment AfterTouch is reachable from the open internet, it needs proper authentication
and authorization, so a passing script kiddie can't read your recently played songs (or
worse). And the docs need some love and a clearer structure. One feature is likely to land
in this stretch too: making
[presets propagate cleanly across the speakers in one account](https://github.com/gesellix/Bose-SoundTouch/issues/495),
without the manual "refresh sources" dance. There's probably more before it's truly
"1.0", but none of it is blocking: there's nothing preventing us from getting there *now*.

It's also a natural moment for a clean slate. If your migration has accumulated quirks,
1.0 is a good excuse to reset and re-migrate your speakers onto a known-good footing.

And then the interesting part begins. With the rescue done and a stable base in place, the
focus shifts to delivering value the old Bose cloud never could. Some of that is already
taking shape in the issue tracker: an
[audiobook mode](https://github.com/gesellix/Bose-SoundTouch/issues/508), and deeper
integration with external music providers such as
[Amazon Music](https://github.com/gesellix/Bose-SoundTouch/issues/188). A service under
community control is a rare chance to actually solve the things people ask for, instead of
waiting on a roadmap that was discontinued. If there's something you wish your speakers
did, the [issue tracker](https://github.com/gesellix/Bose-SoundTouch/issues) and
[Discussions](https://github.com/gesellix/Bose-SoundTouch/discussions) are where it starts.

## Current release

**v0.117.0**, released June 28, 2026

This blog will be updated monthly, or whenever something significant ships.
Subscribe to the [GitHub releases](https://github.com/gesellix/Bose-SoundTouch/releases)
for individual version notes.
