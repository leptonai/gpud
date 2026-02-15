# Install GPUd

To install from the official release:

```bash
curl -fsSL https://pkg.gpud.dev/install.sh | sh
gpud up
```

To install the latest published version explicitly:

```bash
curl -fsSL https://pkg.gpud.dev/install.sh | sh -s $(curl -fsSL https://pkg.gpud.dev/unstable_latest.txt)
```

Then open [localhost:15132](https://localhost:15132) for the local web UI.

## Build

To build and run locally:

```bash
make all
./bin/gpud up
```

To run without systemd (e.g., Mac OS):

```bash
./bin/gpud run
```
