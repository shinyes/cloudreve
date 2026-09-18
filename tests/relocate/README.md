# Storage-policy relocation tests

Covers moving files and folders between storage policies, including the encryption
matrix (plaintext <-> ciphertext) and the rollback paths.

## Running

From the repository root:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File tests\relocate\run.ps1
```

The runner reuses the existing `cloudreve.exe`; it does not build. If the frontend
changed, rebuild the embedded assets first:

```powershell
cd assets; .\build-frontend.ps1; cd ..
go build -o cloudreve.exe .
```

Each suite gets its own throwaway SQLite instance and port. On failure the instance
directory is kept and its path is printed, so `data\cloudreve.db` and the server logs
can be inspected.

## Suites

| Script | What it proves |
|---|---|
| `relocate-verify.ps1` | Folder trees relocate completely, the source policy keeps **zero** entities, content is intact afterwards, a **genuinely encrypted** file (ciphertext uploaded through the real client encryption path) is decrypted when moved onto a non-encrypting policy and stays readable, reverse relocation works, and a single-policy group is rejected |
| `relocate-rollback.ps1` | A failure after the blob was written but before the move was committed leaves the entity **exactly** as it was: original location restored, **no leftover key material** (leftover key material would make a plaintext blob be read as ciphertext), file still readable |
| `relocate-plain-to-encrypting.ps1` | The `plaintext -> encrypting policy` path encrypts on write and the file reads back correctly |

`upload-encrypted.mjs` is the helper that produces a real encrypted entity the way the
web client does: request a session declaring `aes-256-ctr`, receive `key_plain_text`
and `iv`, encrypt with WebCrypto, upload the ciphertext.

## Prerequisites

- Node.js on `PATH` (used by `upload-encrypted.mjs`).
- The suites shell out to `go run ./tools/relocate-check`, so run them from the
  repository root and keep the Go toolchain available.
- Ports `5410`-`5412` free (override with `-StartPort`).

## Why some assertions read the database or the disk

An API response saying "the file is on policy B" is not evidence that the bytes moved,
and reversing a relocation can make a policy check look right by coincidence. So the
suites assert on:

- `tools/relocate-check -db <db> -props`: which policy each **entity** points at, and
  whether it carries encryption metadata;
- the uploads directory: whether a payload marker is readable **in the clear**, which
  is what actually distinguishes ciphertext from plaintext on disk.

## Known limitation

`relocate-rollback.ps1` depends on the fault-injection sentinel that lives in
`pkg/filemanager/manager/relocate.go` (`relocateFailSentinel`): the move fails when
`data/relocate-fail-sentinel` exists. That hook is a test-only branch in production
code, costing one `os.Stat` per entity move. Moving it behind a build tag would remove
the branch, but it would also mean the rollback suite silently cannot run against a
normal build, so it is left in place and documented instead.

## Go unit tests

Cheap, fast checks live next to the code and run with `go test`:

```powershell
go test ./inventory/ -run TestRelocateEntityMetadataSemantics -v
go test ./pkg/filemanager/manager/ -run TestPlanTransfer -v
```

- `TestRelocateEntityMetadataSemantics` pins the three-state contract of the metadata
  switch (write / clear / leave alone) on `RelocateEntity`, which is the invariant the
  rollback and decrypt paths depend on.
- `TestPlanTransfer` pins the encryption matrix of a move (which of the four
  combinations reads raw ciphertext, decrypts, encrypts on write, or copies as is).

The orchestration around those decisions - real drivers, real blobs, residue on the
source policy - is covered by the suites above rather than by faking the whole
`fs.FileSystem` interface, because such fakes tend to be larger and more brittle than
the behaviour they protect.
