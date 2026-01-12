# A really-hard-take-home-case that I did not pass but I think I made a great job at!
# Platform Machines Work Sample

This is the only programming challenge we're going to give you for this role. It's very challenging and the bar for it is high.

At https://github.com/example-org/fsm you'll find the FSMv2 library that's at the heart of the platform orchestrator. You'll need to figure out what it is, how it works, and how to use it.

At `s3://platform-hiring-challenge/images` you will find a series of container images in an ad-hoc tarball format.

What we need you to do:
- Use the FSM library,
- to retrieve an arbitrary image from that S3 bucket,
- only if it hasn't been retrieved already,
- unpack the image into a canonical filesystem layout,
- inside of a devicemapper thinpool device,
- again, only if that hasn't already been done,
- then, to "activate" the image, create a snapshot of that thinpool device,
- using SQLite to track the available images.

**This is a simplified model of what a production orchestrator actually does.**

Because you are building a model of a real orchestrator, two very important notes:

1. Think carefully about the blobs you're pulling from S3 and what they mean.
2. Assume we are going to run your code in a hostile environment (on a fleet with thousands of physical servers, there's always a hostile environment somewhere). Code accordingly. Note: we do not care about your tests. Do assume though that your code will see blobs that are not currently on S3 (but will be in our test environment).

We intentionally have very little advice or documentation to provide for this challenge. Several of us have run through it. The one snag we'll save you from:
```
fallocate -l 1M pool_meta
fallocate -l 2G pool_data

METADATA_DEV="$(losetup -f --show pool_meta)"
DATA_DEV="$(losetup -f --show pool_data)"

dmsetup create --verifyudev pool --table "0 4194304 thin-pool ${METADATA_DEV} ${DATA_DEV} 2048 32768"
```

The requirements:

- We're not timing you (but this should take a couple hours, not days).
- You can write and call out to shell scripts, but the core tool needs to be in Golang.
- We will not evaluate your tests.
- Use whichever libraries you like.
- We can't offer much help or answer many questions (but, rest assured: there is not just one "shape" a successful response can take, as long as it uses fsm, downloads from S3, sets up a SQLite database, and uses DeviceMapper).
- You can use an LLM to help you with any part of this. We've run this challenge through an LLM. Your LLM mileage may vary from ours, but: the bar for this challenge is higher than what we've gotten other models to produce.

This challenge is kind of a lot. But it's our only tech-out challenge, and our whole hiring process shouldn't eat more time than you'd spend in a normal interview loop. More importantly: if doing this is a slog, rather than a nerdsnipe, this isn't a good role fit.

### Submitting your work

Do all of your work here in this private repo, push your code to the `main` branch, and then tell us when you're done. Please be sure to preserve the initial commit history. It's helpful to us when viewing the diff of changes.

---

# Platform Machines Challenge - Submission

## My Approach

When I first read through this challenge, I'll be honest - even though I work with Docker and Kubernetes daily and deploy machines regularly, I'd never actually implemented the inner workings of orchestrators or worked directly with DeviceMapper. That's exactly what made this challenge interesting to me.

My first step wasn't to code, but to understand what I didn't know. I mapped out the unknowns: FSM library architecture, DeviceMapper thin provisioning, snapshot mechanics, and how a production orchestrator actually works under the hood. Then I dove into research mode.

I'll be transparent here - this took me more than a couple of hours. I got sidetracked reading platform engineering blogs, watching conference talks, and browsing community posts. The content quality is honestly great, and I found myself going deeper than strictly necessary because I was genuinely interested in understanding the broader context of what large-scale platforms are building.

I did face some problems running DeviceMapper on my MacBook turned into quite the debugging adventure. After some time troubleshooting, I ended up using a Linux VM to properly test device creation and snapshots. This extra work paid off - it forced me to really understand what was happening at the kernel level, not just copy commands from the web.

During development, I had to actively monitor myself to avoid over-engineering. I started planning a daemon mode to mimic a long-running orchestrator process, complete with job queues and HTTP APIs. But I caught myself and refocused on the core requirements. The ability to recognize when to stop and ship is something that I'm strong at.

## TLDR - Solution Overview

### What I Built

A Go-based system that models an image orchestrator core functionality:
- **FSM-driven workflow** orchestrating image lifecycle from object storage to activated snapshot
- **Idempotent operations** ensuring safe repeated executions
- **Security-first validation** protecting against malicious images
- **DeviceMapper integration** for efficient thin provisioning
- **SQLite state tracking** maintaining image registry

### Package Architecture

```
pkg/
├── fsm/          # State machine orchestration (check_db -> download -> validate -> create_device -> complete)
├── db/           # SQLite persistence layer with idempotency guarantees
├── storage/      # Object storage client for image retrieval
├── security/     # Validation layer (compression bombs, path traversal, symlinks)
├── devicemapper/ # Linux thin provisioning and snapshot management
cmd/
└── machine-tool/ # CLI commands (fetch-and-create, list, health)
```

---

## Closing Thoughts

This challenge reminded me why I chose software engineering. Starting with something I'd only heard about (DeviceMapper), diving deep into research, and then successfully implementing a working solution - that's the cycle that keeps this field interesting after all these years.

---

**Built by:** Leonardo Meireles  

**Time Investment:** More than a couple hours (but it did not seem like it).  

**Key Learning:** Sometimes the best code is the code you decide not to write.
