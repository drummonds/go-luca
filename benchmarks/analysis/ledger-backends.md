# Ledger Backends Benchmark

## Purpose

Compare Ledger performance across backends: MemLedger (pure Go), SQLLedger (pglike/SQLite),
and the HTTP/JSON API layer. This quantifies the cost of each abstraction level so users can
make informed choices between embedding go-luca as a library or running it as a service.

Key questions:
- How fast is MemLedger vs SQLLedger for core operations?
- What is the per-call overhead of the HTTP/JSON API layer?
- How does overhead scale with pre-loaded data volume?

## Analysis

_Run `task bench:api` to generate results, then update this file with analysis._
