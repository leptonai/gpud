# Install GPUd

To install from the official release:

```bash
curl -fsSL https://pkg.gpud.dev/install.sh | sh
gpud up
```

To specify a version

```bash
curl -fsSL https://pkg.gpud.dev/install.sh | sh -s v0.5.1
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
