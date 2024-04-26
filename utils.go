package main

import (
	"fmt"
	"strings"
)

const PARAM_COL = "param"
const VALUE_COL = "value"

func MapToTable(mp map[string]any) ([]string, [][][]rune) {
	var row_cnt = len(mp)
	if row_cnt == 0 {
		return nil, nil
	}

	var headers = make([]string, 2)
	headers[0] = PARAM_COL
	headers[1] = VALUE_COL

	rows := make([][][]rune, 2)
	for i := 0; i < 2; i++ {
		rows[i] = make([][]rune, row_cnt)
	}

	var loc = 0
	for key, obj := range mp {
		rows[0][loc] = []rune(key)
		rows[1][loc] = []rune(fmt.Sprintf("%v", obj))
		loc++
	}

	return headers, rows
}

func MapArrayToTable(mp []map[string]any) ([]string, [][][]rune) {
	var row_cnt = len(mp)
	if row_cnt == 0 {
		return nil, nil
	}

	var headers = make([]string, 0, 32)

	obj := mp[0]

	for key := range obj {
		okk := true
		for j := 1; j < row_cnt; j++ {
			_, ok := mp[j][key]
			if !ok {
				okk = false
				break
			}
		}
		if okk {
			headers = append(headers, key)
		}
	}

	var col_cnt = len(headers)
	if col_cnt == 0 {
		return nil, nil
	}

	rows := make([][][]rune, col_cnt)
	for i := 0; i < col_cnt; i++ {
		rows[i] = make([][]rune, row_cnt)
	}

	for i, obj := range mp {
		for j, key := range headers {
			rows[j][i] = []rune(fmt.Sprintf("%v", obj[key]))
		}
	}

	return headers, rows
}

func FormatTable(header []string, rows [][][]rune) string {
	const MAX_LEN = 16

	table := strings.Builder{}
	cols := make([]int, len(rows))
	for j, col := range header {
		cols[j] = len(col) + 2
	}
	var rowcnt int
	for j, col := range rows {
		rowcnt = len(col)
		for _, cell := range col {
			lv := len(cell)
			if lv > MAX_LEN {
				lv = MAX_LEN
			}
			lv += 2
			if lv > cols[j] {
				cols[j] = lv
			}
		}
	}
	table.WriteString("|")
	for j, col := range header {
		l := len(col)
		s := " " + col + strings.Repeat(" ", cols[j]-l-1)
		table.WriteString(s)
		table.WriteString("|")
	}
	table.WriteString("\n")
	table.WriteString("|")
	for j := range header {
		table.WriteString(strings.Repeat("-", cols[j]))
		table.WriteString("|")
	}
	for i := 0; i < rowcnt; i++ {
		table.WriteString("\n")
		table.WriteString("|")
		for j := range rows {
			cell := rows[j][i]
			l := len(cell)
			table.WriteString(" ")
			if l > MAX_LEN {
				table.WriteString(string(cell[0 : MAX_LEN-3]))
				table.WriteString("...")
			} else {
				table.WriteString(string(cell))
			}
			table.WriteString(strings.Repeat(" ", cols[j]-l-1))
			table.WriteString("|")
		}
	}
	return table.String()
}

func ParseCommand(msg string) (string, []string) {
	var seq []string
	if strings.Contains(msg, "&") {
		seq = strings.Split(msg, "&")
	} else {
		seq = strings.Split(msg, "_")
	}
	if len(seq) > 0 {
		if strings.HasPrefix(seq[0], "/") {
			return seq[0], seq[1:]
		}
	}
	return msg, nil
}

func CollapseParams(params []string) {
	if len(params) > 1 {
		res := ""
		for _, v := range params {
			if len(res) > 0 {
				res += "_"
			}
			res += v
		}
		params[0] = res
	}
}

func ErrorToString(err error) string {
	return fmt.Sprintf("%v", err)
}
