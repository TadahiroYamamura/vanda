# vanda

Toolkit for evidence-first development: keep a list of promises, review and agree on every change, and check that each promise is backed by passing evidence — built for software written by AI agents.

**Status: early development.** Nothing here is usable yet. The first stage (reading the promise list and running checks in CI) is being built.

The name comes from the Quenya word *vanda*, "oath, solemn promise".

## Why

vanda aims to make development three times faster than before, without letting serious bugs or security flaws in. People do not read the large amount of code that AI agents write. They audit the evidence that the code works, and they design the accountability that makes that evidence worth trusting.

vanda is a tool for moving fast without failing, not a tool for proving that software is correct. It comes down to two things:

- **Designing accountability**: a systematic way to design accountability, to see how far each of its parts is achieved, and to improve it.
- **Controlled failure**: anticipating failures as completely as possible, detecting them early, and avoiding fatal situations.

With these in place, AI agents can develop autonomously, people can judge the risk of each change and spend their attention where it is needed, and development becomes both safe and fast.

## Documents

The documents are written in Japanese for now.

- [.vanda/promises.toml](.vanda/promises.toml): the promise list (terms, features, and claims of what vanda does)
- [docs/spec.md](docs/spec.md): features not built yet
- [docs/formats.md](docs/formats.md): file and interface formats

## License

[MIT](LICENSE)
