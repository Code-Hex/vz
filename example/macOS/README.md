Example
=======

You can get knowledge build and codesign process in Makefile.

## Build

```sh
make all
```

## Setup Hints

- I used `ubuntu-20.04.1-live-server-arm64.iso` in this example
- [The Unarchiver](https://apps.apple.com/us/app/the-unarchiver/id425424353?mt=12) can extracts some important files from iso.
    - `vmlinuz` and `initrd` in `/casper`
    - Need to rename vmlinuz to vmlinuz.gz and unarchive it.
- https://forums.macrumors.com/threads/ubuntu-linux-virtualized-on-m1-success.2270365/