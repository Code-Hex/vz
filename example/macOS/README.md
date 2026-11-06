Example
=======

You can get knowledge build and codesign process in Makefile.

## Build

```sh
make all
```

## Run

- `./virtualization -install` install macOS to your VM.
- `./virtualization` run macOS VM.

## Provisioning on First Boot

MacOS 27 and later supports guest provisioning options on first boot. When this
example runs under a supported version, it will provision the VM with a user
named `vzuser`, a password `guest-password` and with remote ssh enabled. This
happens at run time, not install time.

The hostname is predictable and will be reachable on mDNS.

```sh
ssh vzuser@Guests-Virtual-Machine.local
# use guest-password to log in
```

## Resources

Some resources will be created in `VM.bundle` directory on your home directory.