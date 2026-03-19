module github.com/drummonds/go-luca

go 1.26.0

require (
	github.com/drummonds/go-postgres v0.5.0
	github.com/drummonds/gotreesitter v0.6.1
	github.com/google/uuid v1.6.0
	github.com/jackc/pgx/v5 v5.8.0
	github.com/shopspring/decimal v1.4.0
	github.com/yuin/goldmark v1.7.16
)

require (
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/ncruces/go-sqlite3 v0.32.0 // indirect
	github.com/ncruces/julianday v1.0.0 // indirect
	github.com/tetratelabs/wazero v1.11.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.41.0 // indirect
	golang.org/x/text v0.34.0 // indirect
)

replace github.com/drummonds/gotreesitter v0.6.1 => /home/hum3/nibble/gotreesitter

replace github.com/ncruces/go-sqlite3 v0.32.0 => github.com/ncruces/go-sqlite3 v0.30.6-0.20260318175627-361fdc52faa5
