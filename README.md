# pylingual-cli

A small CLI for sending `.pyc` files to the Pylingual API and writing the decompiled Python output locally.

## Build

```sh
make build
```

The binary is written to:

```sh
bin/pylingual
```

## Install

Copy the built binary into `/usr/local/bin`:

```sh
sudo cp bin/pylingual /usr/local/bin/pylingual
sudo chmod +x /usr/local/bin/pylingual
```

Confirm it is available:

```sh
pylingual -h
```

## Usage

Decompile one file:

```sh
pylingual sample.pyc
```

Decompile a directory recursively and write output under `out/`:

```sh
pylingual -o out path/to/pyc-files
```

Use plain line-based output for logs or scripts:

```sh
pylingual --plain -o out path/to/pyc-files
```

## Development

```sh
make test
make vet
make race
make check
```
