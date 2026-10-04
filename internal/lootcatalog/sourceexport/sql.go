// Package sourceexport lexically reads upstream dumps; it never executes SQL.
package sourceexport

import (
	"fmt"
	"strings"
)

type token struct {
	value  string
	quoted bool
}
type lexer struct {
	input    string
	position int
}

func (l *lexer) next() (token, error) {
	for l.position < len(l.input) {
		c := l.input[l.position]
		if c == ' ' || c == '\r' || c == '\n' || c == '\t' {
			l.position++
			continue
		}
		if strings.HasPrefix(l.input[l.position:], "--") || c == '#' {
			end := strings.IndexByte(l.input[l.position:], '\n')
			if end < 0 {
				l.position = len(l.input)
			} else {
				l.position += end + 1
			}
			continue
		}
		if strings.HasPrefix(l.input[l.position:], "/*") {
			end := strings.Index(l.input[l.position+2:], "*/")
			if end < 0 {
				return token{}, fmt.Errorf("unterminated SQL comment")
			}
			l.position += end + 4
			continue
		}
		if c == '\'' || c == '`' || c == '"' {
			quote := c
			l.position++
			var value strings.Builder
			for l.position < len(l.input) {
				c = l.input[l.position]
				l.position++
				if c == quote {
					if l.position < len(l.input) && l.input[l.position] == quote {
						value.WriteByte(quote)
						l.position++
						continue
					}
					return token{value.String(), true}, nil
				}
				if c == '\\' && quote != '`' {
					if l.position >= len(l.input) {
						return token{}, fmt.Errorf("unterminated SQL escape")
					}
					c = l.input[l.position]
					l.position++
					switch c {
					case 'n':
						c = '\n'
					case 'r':
						c = '\r'
					case 't':
						c = '\t'
					case '0':
						c = 0
					case 'b':
						c = '\b'
					case 'Z':
						c = 26
					}
				}
				value.WriteByte(c)
			}
			return token{}, fmt.Errorf("unterminated SQL string")
		}
		if strings.ContainsRune("(),;=", rune(c)) {
			l.position++
			return token{value: string(c)}, nil
		}
		start := l.position
		for l.position < len(l.input) && !strings.ContainsRune(" \r\n\t(),;='\"`", rune(l.input[l.position])) {
			l.position++
		}
		return token{value: l.input[start:l.position]}, nil
	}
	return token{}, nil
}

// Parse reads the named CREATE TABLE column order and INSERT VALUES tuples.
// Each row is passed to visit immediately so unrelated world rows need not be retained.
func Parse(input, table string, visit func(map[string]string) error) error {
	l := lexer{input: input}
	columns := []string{}
	hasInsert := false
	next := func() (token, error) { return l.next() }
	for {
		t, err := next()
		if err != nil {
			return err
		}
		if t.value == "" && !t.quoted {
			break
		}
		if strings.EqualFold(t.value, "CREATE") && !t.quoted {
			t, err = next()
			if err != nil {
				return err
			}
			if !strings.EqualFold(t.value, "TABLE") {
				continue
			}
			t, err = next()
			if err != nil {
				return err
			}
			if t.value != table {
				continue
			}
			t, err = next()
			if err != nil {
				return err
			}
			if t.value != "(" {
				return fmt.Errorf("unsupported CREATE TABLE %s", table)
			}
			depth, atStart := 1, true
			for depth > 0 {
				t, err = next()
				if err != nil {
					return err
				}
				if t.value == "" && !t.quoted {
					return fmt.Errorf("truncated CREATE TABLE")
				}
				if depth == 1 && atStart && t.quoted {
					columns = append(columns, strings.ToLower(t.value))
				}
				atStart = false
				if !t.quoted {
					switch t.value {
					case "(":
						depth++
					case ")":
						depth--
					case ",":
						if depth == 1 {
							atStart = true
						}
					}
				}
			}
		}
		if !strings.EqualFold(t.value, "INSERT") || t.quoted {
			continue
		}
		t, err = next()
		if err != nil {
			return err
		}
		if !strings.EqualFold(t.value, "INTO") {
			return fmt.Errorf("unsupported INSERT")
		}
		t, err = next()
		if err != nil {
			return err
		}
		if t.value != table {
			return fmt.Errorf("unexpected INSERT table %q in %s dump", t.value, table)
		}
		t, err = next()
		if err != nil {
			return err
		}
		insertColumns := columns
		if t.value == "(" && !t.quoted {
			insertColumns = []string{}
			seen := map[string]bool{}
			for {
				column, err := next()
				if err != nil {
					return err
				}
				name := strings.ToLower(column.value)
				if !column.quoted || name == "" || seen[name] {
					return fmt.Errorf("invalid INSERT column")
				}
				seen[name] = true
				insertColumns = append(insertColumns, name)
				separator, err := next()
				if err != nil {
					return err
				}
				if separator.value == ")" && !separator.quoted {
					break
				}
				if separator.value != "," || separator.quoted {
					return fmt.Errorf("invalid INSERT column separator")
				}
			}
			t, err = next()
			if err != nil {
				return err
			}
		}
		if !strings.EqualFold(t.value, "VALUES") || len(insertColumns) == 0 {
			return fmt.Errorf("INSERT must use declared column order for %s", table)
		}
		hasInsert = true
		for {
			t, err = next()
			if err != nil {
				return err
			}
			if t.value != "(" || t.quoted {
				return fmt.Errorf("expected SQL tuple")
			}
			row := make(map[string]string, len(insertColumns))
			for index, column := range insertColumns {
				value, err := next()
				if err != nil {
					return err
				}
				if value.value == "" && !value.quoted {
					return fmt.Errorf("truncated tuple")
				}
				if !value.quoted && strings.ContainsAny(value.value, "(),;") {
					return fmt.Errorf("invalid tuple value")
				}
				row[column] = value.value
				separator, err := next()
				if err != nil {
					return err
				}
				expected := ","
				if index == len(insertColumns)-1 {
					expected = ")"
				}
				if separator.value != expected || separator.quoted {
					return fmt.Errorf("wrong column count in %s", table)
				}
			}
			if err := visit(row); err != nil {
				return err
			}
			t, err = next()
			if err != nil {
				return err
			}
			if t.value == ";" && !t.quoted {
				break
			}
			if t.value != "," || t.quoted {
				return fmt.Errorf("expected SQL tuple separator")
			}
		}
	}
	if !hasInsert {
		return fmt.Errorf("no INSERT VALUES for %s", table)
	}
	return nil
}
