package luca

import (
	"fmt"
	"io"
	"strings"

	"codeberg.org/hum3/gotreesitter"
	"codeberg.org/hum3/gotreesitter/grammars"
)

// Transaction is an in-memory representation of a .goluca transaction,
// independent of the database.
type Transaction struct {
	DateTime          DateTime
	KnowledgeDateTime *DateTime // %datetime — when the transaction was known
	Flag              rune      // '*' posted, '!' pending
	Payee             string
	Movements         []TextMovement
	Metadata          map[string]string // key → value
}

// TextMovement is a single movement line in a .goluca file.
type TextMovement struct {
	Linked      bool   // '+' prefix
	From        string // account full path
	To          string // account full path
	Arrow       string // "->", "//", "→", ">" — preserved for round-trip
	Description string // without quotes
	Amount      string // decimal string as written (e.g. "1,000.00")
	Commodity   string // e.g. "GBP"
}

// Option is a key-value setting from an option directive.
type Option struct {
	Key   string
	Value string
}

// AliasDef maps a short name to an account path.
type AliasDef struct {
	Name    string
	Account string
}

// CommodityDef defines a commodity with optional metadata.
type CommodityDef struct {
	DateTime *DateTime
	Code     string
	Metadata map[string]string
}

// OpenDef declares an account opening.
type OpenDef struct {
	DateTime    DateTime
	Account     string
	Commodities []string
	Metadata    map[string]string
}

// CustomerDef defines a customer with account and constraints.
type CustomerDef struct {
	Name                string
	Account             string
	MaxBalanceAmount    string
	MaxBalanceCommodity string
	Metadata            map[string]string
}

// DataPoint is a time-series parameter value.
type DataPoint struct {
	DateTime          DateTime
	KnowledgeDateTime *DateTime
	ParamName         string
	ParamValue        string
}

// GolucaFile is an in-memory representation of a .goluca file.
type GolucaFile struct {
	Options      []Option
	Commodities  []CommodityDef
	Opens        []OpenDef
	Aliases      []AliasDef
	Customers    []CustomerDef
	DataPoints   []DataPoint
	Transactions []Transaction
}

// ParseGoluca parses .goluca formatted text into a GolucaFile.
func ParseGoluca(r io.Reader) (*GolucaFile, error) {
	src, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read source: %w", err)
	}

	lang := grammars.GolucaLanguage()
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
		switch child.Type(lang) {
		case "transaction":
			txn, err := parseTransaction(child, src, lang)
			if err != nil {
				return nil, err
			}
			gf.Transactions = append(gf.Transactions, txn)
		case "directive":
			if err := parseDirective(child, src, lang, &gf); err != nil {
				return nil, err
			}
		}
	}
	return &gf, nil
}

func parseTransaction(n *gotreesitter.Node, src []byte, lang *gotreesitter.Language) (Transaction, error) {
	var txn Transaction
	hasLinked := false
	for i := range n.ChildCount() {
		child := n.Child(i)
		switch child.Type(lang) {
		case "header":
			parseHeader(child, src, lang, &txn)
		case "movement":
			m := parseMovement(child, src, lang)
			if m.Linked {
				hasLinked = true
			}
			txn.Movements = append(txn.Movements, m)
		case "metadata_line":
			if txn.Metadata == nil {
				txn.Metadata = make(map[string]string)
			}
			key, value := parseMetadataLine(child, src, lang)
			txn.Metadata[key] = value
		}
	}
	if hasLinked {
		for i := range txn.Movements {
			txn.Movements[i].Linked = true
		}
	}
	return txn, nil
}

func parseHeader(n *gotreesitter.Node, src []byte, lang *gotreesitter.Language, txn *Transaction) {
	for i := range n.ChildCount() {
		child := n.Child(i)
		switch child.Type(lang) {
		case "datetime":
			txn.DateTime = parseDateTimeNode(child, src, lang)
		case "knowledge_datetime":
			for j := range child.ChildCount() {
				kc := child.Child(j)
				if kc.Type(lang) == "datetime" {
					kdt := parseDateTimeNode(kc, src, lang)
					txn.KnowledgeDateTime = &kdt
				}
			}
		case "flag":
			text := child.Text(src)
			if len(text) > 0 {
				txn.Flag = rune(text[0])
			}
		case "payee":
			txn.Payee = strings.TrimSpace(child.Text(src))
		case "date":
			// Backward compat: old grammar without datetime wrapper
			txn.DateTime = DateTime{Date: child.Text(src)}
		}
	}
}

func parseDateTimeNode(n *gotreesitter.Node, src []byte, lang *gotreesitter.Language) DateTime {
	var dt DateTime
	for i := range n.ChildCount() {
		child := n.Child(i)
		switch child.Type(lang) {
		case "date":
			dt.Date = child.Text(src)
		case "time":
			dt.Time = child.Text(src)
		case "fractional":
			dt.Fractional = child.Text(src)
		case "timezone":
			dt.Timezone = child.Text(src)
		case "period_anchor":
			dt.PeriodAnchor = child.Text(src)
		}
	}
	return dt
}

func parseMovement(n *gotreesitter.Node, src []byte, lang *gotreesitter.Language) TextMovement {
	var m TextMovement
	for i := range n.ChildCount() {
		child := n.Child(i)
		switch child.Type(lang) {
		case "linked_prefix":
			m.Linked = true
		case "arrow":
			m.Arrow = child.Text(src)
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

func parseMetadataLine(n *gotreesitter.Node, src []byte, lang *gotreesitter.Language) (string, string) {
	var key, value string
	for i := range n.ChildCount() {
		child := n.Child(i)
		switch child.Type(lang) {
		case "metadata_key":
			key = child.Text(src)
		case "metadata_value":
			value = strings.TrimSpace(child.Text(src))
		}
	}
	return key, value
}

func parseDirective(n *gotreesitter.Node, src []byte, lang *gotreesitter.Language, gf *GolucaFile) error {
	for i := range n.ChildCount() {
		child := n.Child(i)
		switch child.Type(lang) {
		case "option_directive":
			parseOptionDirective(child, src, lang, gf)
		case "alias_directive":
			parseAliasDirective(child, src, lang, gf)
		case "commodity_directive":
			parseCommodityDirective(child, src, lang, gf)
		case "open_directive":
			parseOpenDirective(child, src, lang, gf)
		case "customer_directive":
			parseCustomerDirective(child, src, lang, gf)
		case "data_point":
			parseDataPointDirective(child, src, lang, gf)
		}
	}
	return nil
}

func parseOptionDirective(n *gotreesitter.Node, src []byte, lang *gotreesitter.Language, gf *GolucaFile) {
	var opt Option
	for i := range n.ChildCount() {
		child := n.Child(i)
		switch child.Type(lang) {
		case "option_key":
			opt.Key = child.Text(src)
		case "option_value":
			opt.Value = strings.TrimSpace(child.Text(src))
		}
	}
	gf.Options = append(gf.Options, opt)
}

func parseAliasDirective(n *gotreesitter.Node, src []byte, lang *gotreesitter.Language, gf *GolucaFile) {
	var alias AliasDef
	for i := range n.ChildCount() {
		child := n.Child(i)
		switch child.Type(lang) {
		case "alias_name":
			alias.Name = child.Text(src)
		case "account":
			alias.Account = child.Text(src)
		}
	}
	gf.Aliases = append(gf.Aliases, alias)
}

func parseCommodityDirective(n *gotreesitter.Node, src []byte, lang *gotreesitter.Language, gf *GolucaFile) {
	var cd CommodityDef
	for i := range n.ChildCount() {
		child := n.Child(i)
		switch child.Type(lang) {
		case "datetime":
			dt := parseDateTimeNode(child, src, lang)
			cd.DateTime = &dt
		case "commodity":
			cd.Code = child.Text(src)
		case "metadata_line":
			if cd.Metadata == nil {
				cd.Metadata = make(map[string]string)
			}
			key, value := parseMetadataLine(child, src, lang)
			cd.Metadata[key] = value
		}
	}
	gf.Commodities = append(gf.Commodities, cd)
}

func parseOpenDirective(n *gotreesitter.Node, src []byte, lang *gotreesitter.Language, gf *GolucaFile) {
	var od OpenDef
	for i := range n.ChildCount() {
		child := n.Child(i)
		switch child.Type(lang) {
		case "datetime":
			od.DateTime = parseDateTimeNode(child, src, lang)
		case "account":
			od.Account = child.Text(src)
		case "commodity_list":
			for j := range child.ChildCount() {
				cc := child.Child(j)
				if cc.Type(lang) == "commodity" {
					od.Commodities = append(od.Commodities, cc.Text(src))
				}
			}
		case "metadata_line":
			if od.Metadata == nil {
				od.Metadata = make(map[string]string)
			}
			key, value := parseMetadataLine(child, src, lang)
			od.Metadata[key] = value
		}
	}
	gf.Opens = append(gf.Opens, od)
}

func parseCustomerDirective(n *gotreesitter.Node, src []byte, lang *gotreesitter.Language, gf *GolucaFile) {
	var cd CustomerDef
	for i := range n.ChildCount() {
		child := n.Child(i)
		switch child.Type(lang) {
		case "customer_name":
			cd.Name = strings.Trim(child.Text(src), "\"")
		case "customer_property":
			for j := range child.ChildCount() {
				prop := child.Child(j)
				switch prop.Type(lang) {
				case "customer_account":
					for k := range prop.ChildCount() {
						ac := prop.Child(k)
						if ac.Type(lang) == "account" {
							cd.Account = ac.Text(src)
						}
					}
				case "customer_constraint":
					for k := range prop.ChildCount() {
						cc := prop.Child(k)
						switch cc.Type(lang) {
						case "amount":
							cd.MaxBalanceAmount = cc.Text(src)
						case "commodity":
							cd.MaxBalanceCommodity = cc.Text(src)
						}
					}
				case "metadata_line":
					if cd.Metadata == nil {
						cd.Metadata = make(map[string]string)
					}
					key, value := parseMetadataLine(prop, src, lang)
					cd.Metadata[key] = value
				}
			}
		}
	}
	gf.Customers = append(gf.Customers, cd)
}

func parseDataPointDirective(n *gotreesitter.Node, src []byte, lang *gotreesitter.Language, gf *GolucaFile) {
	var dp DataPoint
	for i := range n.ChildCount() {
		child := n.Child(i)
		switch child.Type(lang) {
		case "datetime":
			dp.DateTime = parseDateTimeNode(child, src, lang)
		case "knowledge_datetime":
			for j := range child.ChildCount() {
				kc := child.Child(j)
				if kc.Type(lang) == "datetime" {
					kdt := parseDateTimeNode(kc, src, lang)
					dp.KnowledgeDateTime = &kdt
				}
			}
		case "param_name":
			dp.ParamName = child.Text(src)
		case "param_value":
			dp.ParamValue = strings.TrimSpace(child.Text(src))
		}
	}
	gf.DataPoints = append(gf.DataPoints, dp)
}

// WriteTo writes the GolucaFile in .goluca format.
func (gf *GolucaFile) WriteTo(w io.Writer) (int64, error) {
	var total int64
	needBlank := false

	// Options
	for _, opt := range gf.Options {
		n, err := fmt.Fprintf(w, "option %s %s\n", opt.Key, opt.Value)
		total += int64(n)
		if err != nil {
			return total, err
		}
		needBlank = true
	}

	// Commodities
	for _, c := range gf.Commodities {
		if needBlank {
			n, err := fmt.Fprint(w, "\n")
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
		if c.DateTime != nil {
			n, err := fmt.Fprintf(w, "%s commodity %s\n", c.DateTime.String(), c.Code)
			total += int64(n)
			if err != nil {
				return total, err
			}
		} else {
			n, err := fmt.Fprintf(w, "commodity %s\n", c.Code)
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
		for key, value := range c.Metadata {
			n, err := fmt.Fprintf(w, "  %s: %s\n", key, value)
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
		needBlank = true
	}

	// Opens
	for _, o := range gf.Opens {
		if needBlank {
			n, err := fmt.Fprint(w, "\n")
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
		commodities := strings.Join(o.Commodities, ",")
		if commodities != "" {
			n, err := fmt.Fprintf(w, "%s open %s %s\n", o.DateTime.String(), o.Account, commodities)
			total += int64(n)
			if err != nil {
				return total, err
			}
		} else {
			n, err := fmt.Fprintf(w, "%s open %s\n", o.DateTime.String(), o.Account)
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
		for key, value := range o.Metadata {
			n, err := fmt.Fprintf(w, "  %s: %s\n", key, value)
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
		needBlank = true
	}

	// Aliases
	for _, a := range gf.Aliases {
		if needBlank {
			n, err := fmt.Fprint(w, "\n")
			total += int64(n)
			if err != nil {
				return total, err
			}
			needBlank = false
		}
		n, err := fmt.Fprintf(w, "alias %s %s\n", a.Name, a.Account)
		total += int64(n)
		if err != nil {
			return total, err
		}
	}

	// Data points
	for _, dp := range gf.DataPoints {
		if needBlank {
			n, err := fmt.Fprint(w, "\n")
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
		var sb strings.Builder
		sb.WriteString(dp.DateTime.String())
		if dp.KnowledgeDateTime != nil {
			sb.WriteString("%")
			sb.WriteString(dp.KnowledgeDateTime.String())
		}
		n, err := fmt.Fprintf(w, "%s data %s %s\n", sb.String(), dp.ParamName, dp.ParamValue)
		total += int64(n)
		if err != nil {
			return total, err
		}
		needBlank = true
	}

	// Customers
	for _, c := range gf.Customers {
		if needBlank {
			n, err := fmt.Fprint(w, "\n")
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
		n, err := fmt.Fprintf(w, "customer \"%s\"\n", c.Name)
		total += int64(n)
		if err != nil {
			return total, err
		}
		if c.Account != "" {
			n, err := fmt.Fprintf(w, "  account %s\n", c.Account)
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
		if c.MaxBalanceAmount != "" {
			n, err := fmt.Fprintf(w, "  max-aggregate-balance %s %s\n", c.MaxBalanceAmount, c.MaxBalanceCommodity)
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
		for key, value := range c.Metadata {
			n, err := fmt.Fprintf(w, "  %s: %s\n", key, value)
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
		needBlank = true
	}

	// Transactions
	for i, txn := range gf.Transactions {
		if i > 0 || needBlank {
			n, err := fmt.Fprint(w, "\n")
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
		// Header
		var headerSB strings.Builder
		headerSB.WriteString(txn.DateTime.String())
		if txn.KnowledgeDateTime != nil {
			headerSB.WriteString("%")
			headerSB.WriteString(txn.KnowledgeDateTime.String())
		}
		headerSB.WriteString(" ")
		headerSB.WriteString(string(txn.Flag))
		if txn.Payee != "" {
			headerSB.WriteString(" ")
			headerSB.WriteString(txn.Payee)
		}
		headerSB.WriteString("\n")
		n, err := fmt.Fprint(w, headerSB.String())
		total += int64(n)
		if err != nil {
			return total, err
		}

		// Movements
		for _, m := range txn.Movements {
			var sb strings.Builder
			sb.WriteString("  ")
			if m.Linked {
				sb.WriteString("+")
			}
			sb.WriteString(m.From)
			sb.WriteString(" ")
			arrow := m.Arrow
			if arrow == "" {
				arrow = "→"
			}
			sb.WriteString(arrow)
			sb.WriteString(" ")
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

		// Metadata
		for key, value := range txn.Metadata {
			n, err := fmt.Fprintf(w, "  %s: %s\n", key, value)
			total += int64(n)
			if err != nil {
				return total, err
			}
		}
	}
	return total, nil
}
