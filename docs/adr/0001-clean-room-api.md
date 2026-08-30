---
status: accepted
---

# Build a clean-room API

The project will implement a new public API rather than preserve compatibility with `github.com/valyala/bytebufferpool`, because compatibility would retain its implicit sizing and weak ownership boundary. Public implementations may inform the design, but code will be independently written; non-obvious borrowed ideas will carry adjacent source references and documented differences.
