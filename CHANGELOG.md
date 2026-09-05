# Changelog

All notable changes to local-mind are documented here.

## [0.2.0] - 2026-09-05

### Documentation
- Reconcile ROADMAP with shipped M5 distribution work
([ce1e2ee](https://github.com/AgusRdz/local-mind/commit/ce1e2ee75ed39b08f50623992d0f987e912cc5b5))

### Features
- Add links command to surface dangling [[wikilinks]]
([29b6b6d](https://github.com/AgusRdz/local-mind/commit/29b6b6dca55e9a760b7df6fd289fab261abade27))
## [0.1.5] - 2026-09-03

### Bug Fixes
- Cap oversized body to budget; gate body band on >=2 matched terms
([c8859ca](https://github.com/AgusRdz/local-mind/commit/c8859caffe8c176f2fed753c619bfb4ce65528d5))
## [0.1.4] - 2026-09-03

### Features
- Batch suggest-aliases --all to backfill the whole vault
([f9e47f2](https://github.com/AgusRdz/local-mind/commit/f9e47f27792f4fb7c15a9f3269b7581be28dc4e5))
## [0.1.3] - 2026-09-03

### Features
- Add suggest-aliases (offline alias generation via claude CLI)
([7b31104](https://github.com/AgusRdz/local-mind/commit/7b311040a1e3c9d2e5db7cc35c9711aab32c6bd7))
## [0.1.2] - 2026-09-03

### Bug Fixes
- Drop duplicate 'v' prefix (version already includes it)
([047e610](https://github.com/AgusRdz/local-mind/commit/047e610362a2296ce7624b3bdef68912428fc491))

### Features
- Add self-update command (local-mind update)
([1ca53fa](https://github.com/AgusRdz/local-mind/commit/1ca53fa6c7cf415af90df97aac32659a7e9412f9))
## [0.1.1] - 2026-09-03

### Bug Fixes
- Verify signature against exact checksums file
([ff52168](https://github.com/AgusRdz/local-mind/commit/ff52168939d21f2734e27b8c2089efa0cfb5f1f3))
- Don't crash on empty $PSScriptRoot when run via irm | iex
([c08592c](https://github.com/AgusRdz/local-mind/commit/c08592cab0755228c52c225f6896d1075fc26068))

### Features
- Add doctor, config, and richer stats + bad feedback (M3)
([118cef4](https://github.com/AgusRdz/local-mind/commit/118cef40520195dcf8388a50e37b0e69f9264381))
## [0.1.0] - 2026-09-03

### CI/CD
- Add CI and release workflows
([66ea81f](https://github.com/AgusRdz/local-mind/commit/66ea81f62e80731c711ed130b175ebdf66c62c23))

### Documentation
- Expand README to chop parity; anonymous install for public repo
([10e3f53](https://github.com/AgusRdz/local-mind/commit/10e3f53e68e8e0e955290e87cfab303f5b514906))

### Features
- Initial retrieval bridge (M0-M2)
([a34ab6c](https://github.com/AgusRdz/local-mind/commit/a34ab6cca85e08d6cced01b332f2e619d4053796))
- Add install scripts + release pipeline (M5)
([774780b](https://github.com/AgusRdz/local-mind/commit/774780b7b7d82cef7e4702466653cbbc87d53915))

### Miscellaneous
- Add release signing public key
([db2c689](https://github.com/AgusRdz/local-mind/commit/db2c6895e0bae664b53b6dca9627e4eb0b93a185))

