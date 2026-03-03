package luca

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/odvcencio/gotreesitter"

	"github.com/drummonds/go-luca/internal/golucagrammar"
)

// Transaction is an in-memory representation of a .goluca transaction,
// independent of the database.
type Transaction struct {
	Date      time.Time
	Flag      rune // '*' posted, '!' pending
	Payee     string
	Movements []TextMovement
}

// TextMovement is a single movement line in a .goluca file.
type TextMovement struct {
	Linked      bool   // '+' prefix
	From        string // account full path
	To          string // account full path
	Description string // without quotes
	Amount      string // decimal string as written (e.g. "1,000.00")
	Commodity   string // e.g. "GBP"
}

// GolucaFile is an in-memory representation of a .goluca file.
type GolucaFile struct {
	Transactions []Transaction
}

// ParseGoluca parses .goluca formatted text into a GolucaFile.
func ParseGoluca(r io.Reader) (*GolucaFile, error) {
	src, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}

	lang := golucagrammar.GolucaLanguage()
	parser := gotreesitter.NewParser(lang)
	tree, err := parser.Parse(src)
	if err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}
	root := tree.RootNode()
	if root == nil {
		return &GolucaFile{}, nil
	}
	if root.HasError() {
		return nil, fmt.Errorf("parse error in .goluca input")
	}

	var gf GolucaFile
	for i := range root.ChildCount() {
		child := root.Child(i)
		if child.Type(lang) != "transaction" {
			continue
		}
		txn, err := parseTransaction(child, src, lang)
		if err != nil {
			return nil, err
		}
		gf.Transactions = append(gf.Transactions, txn)
	}
	return &gf, nil
}

func parseTransaction(n *gotreesitter.Node, src []byte, lang *gotreesitter.Language) (Transaction, error) {
	var txn Transaction
	for i := range n.ChildCount() {
		child := n.Child(i)
		switch child.Type(lang) {
		case "header":
			parseHeader(child, src, lang, &txn)
		case "movement":
			m := parseMovement(child, src, lang)
			txn.Movements = append(txn.Movements, m)
		}
	}
	return txn, nil
}

func parseHeader(n *gotreesitter.Node, src []byte, lang *gotreesitter.Language, txn *Transaction) {
	for i := range n.ChildCount() {
		child := n.Child(i)
		switch child.Type(lang) {
		case "date":
			txn.Date, _ = time.Parse("2006-01-02", child.Text(src))
		case "flag":
			text := child.Text(src)
			if len(text) > 0 {
				txn.Flag = rune(text[0])
			}
		case "payee":
			txn.Payee = strings.TrimSpace(child.Text(src))
		}
	}
}

func parseMovement(n *gotreesitter.Node, src []byte, lang *gotreesitter.Language) TextMovement {
	var m TextMovement
	for i := range n.ChildCount() {
		child := n.Child(i)
		switch child.Type(lang) {
		case "linked_prefix":
			m.Linked = true
		case "description":
			text := child.Text(src)
			m.Description = strings.Trim(text, "\"")
		case "amount":
			m.Amount = child.Text(src)
		case "commodity":
			m.Commodity = child.Text(src)
		case "account":
			fieldName := n.FieldNameForChild(i, lang)
			switch fieldName {
			case "from":
				m.From = child.Text(src)
			case "to":
				m.To = child.Text(src)
			}
		}
	}
	return m
}

// WriteTo writes the GolucaFile in .goluca format.
func (gf *GolucaFile) WriteTo(w io.Writer) (int64, error) {
	var total int64
	for i, txn := range gf.Transactions {
		if i > 0 {
			n, err := fmt.Fprint(w, "\n")
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
		// Header
		flag := string(txn.Flag)
		if txn.Payee != "" {
			n, err := fmt.Fprintf(w, "%s %s %s\n", txn.Date.Format("2006-01-02"), flag, txn.Payee)
			total += int64(n)
			if err != nil {
				return total, err
			}
		} else {
			n, err := fmt.Fprintf(w, "%s %s\n", txn.Date.Format("2006-01-02"), flag)
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
		// Movements
		for _, m := range txn.Movements {
			var sb strings.Builder
			sb.WriteString("  ")
			if m.Linked {
				sb.WriteString("+")
			}
			sb.WriteString(m.From)
			sb.WriteString(" → ")
			sb.WriteString(m.To)
			if m.Description != "" {
				sb.WriteString(" \"")
				sb.WriteString(m.Description)
				sb.WriteString("\"")
			}
			// Strip commas from amount
			amt := strings.ReplaceAll(m.Amount, ",", "")
			sb.WriteString(" ")
			sb.WriteString(amt)
			sb.WriteString(" ")
			sb.WriteString(m.Commodity)
			sb.WriteString("\n")
			n, err := fmt.Fprint(w, sb.String())
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
	}
	return total, nil
}
