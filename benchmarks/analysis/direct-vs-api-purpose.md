Measure the overhead of the HTTP/JSON API layer compared to direct in-process method calls
on the Ledger interface. This quantifies the cost of decoupling via the API so users can
make informed choices between embedding go-luca as a library or running it as a service.

Key questions:
- What is the per-call overhead for write operations (RecordMovement)?
- What is the per-call overhead for read operations (Balance)?
- How does overhead scale with pre-loaded data volume?
