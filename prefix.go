package luca

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// LedgerOption configures a SQLLedger as NewSQLLedger builds it.
type LedgerOption func(*SQLLedger) error

// Prefix names every table, index and contract view of the ledger so
// several ledgers share one database (a general ledger beside the
// customer sub-ledger): <prefix>movements, idx_<prefix>movements_from,
// contract_<prefix>ledger_movements. A prefix is lower-case letters,
// digits and underscores starting with a letter, "gl_" say. Without one
// the ledger's names are as they always were, so an existing database
// needs no migration.
func Prefix(prefix string) LedgerOption {
	return func(l *SQLLedger) error {
		if !prefixForm.MatchString(prefix) {
			return fmt.Errorf("prefix %q: want lower-case letters, digits and underscores, starting with a letter", prefix)
		}
		l.names = &names{prefix: prefix}
		return nil
	}
}

var prefixForm = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// names rewrites the ledger's SQL so its tables, indexes and views carry
// the prefix. Every statement the ledger runs passes through it, so the
// SQL in the ledger is written once, unprefixed, and the rewrite is
// cached per statement.
type names struct {
	prefix string
	cache  sync.Map // statement → rewritten statement
}

// The names the schema creates, read off SchemaSQL so a new table or
// view is picked up without a second list. Contract views are prefixed
// after their contract_ marker, everything else in front.
var (
	tableName    *regexp.Regexp
	indexName    = regexp.MustCompile(`\bidx_`)
	contractView = regexp.MustCompile(`\bcontract_ledger_`)
)

func init() {
	created := regexp.MustCompile(`CREATE (?:TABLE|VIEW)(?: IF NOT EXISTS)? (\w+)`)
	var plain []string
	for _, m := range created.FindAllStringSubmatch(SchemaSQL, -1) {
		if !strings.HasPrefix(m[1], "contract_ledger_") {
			plain = append(plain, m[1])
		}
	}
	tableName = regexp.MustCompile(`\b(` + strings.Join(plain, "|") + `)\b`)
}

func (n *names) sql(q string) string {
	if n == nil {
		return q
	}
	if r, ok := n.cache.Load(q); ok {
		return r.(string)
	}
	r := tableName.ReplaceAllString(q, n.prefix+"$1")
	r = indexName.ReplaceAllString(r, "idx_"+n.prefix)
	r = contractView.ReplaceAllString(r, "contract_"+n.prefix+"ledger_")
	n.cache.Store(q, r)
	return r
}

// prefixed is a dbtx that rewrites each statement's names before running
// it on the connection or transaction beneath.
type prefixed struct {
	inner dbtx
	names *names
}

func (p prefixed) Exec(query string, args ...any) (sql.Result, error) {
	return p.inner.Exec(p.names.sql(query), args...)
}

func (p prefixed) Query(query string, args ...any) (*sql.Rows, error) {
	return p.inner.Query(p.names.sql(query), args...)
}

func (p prefixed) QueryRow(query string, args ...any) *sql.Row {
	return p.inner.QueryRow(p.names.sql(query), args...)
}

// wrap binds d to the ledger's names; an unprefixed ledger runs on d as is.
func (l *SQLLedger) wrap(d dbtx) dbtx {
	if l.names == nil {
		return d
	}
	return prefixed{inner: d, names: l.names}
}

// unwrap is the connection or transaction beneath the ledger's names.
func unwrap(d dbtx) dbtx {
	if p, ok := d.(prefixed); ok {
		return p.inner
	}
	return d
}
