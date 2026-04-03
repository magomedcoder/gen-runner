package spreadsheet

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/magomedcoder/gen/pkg/document"
	"github.com/xuri/excelize/v2"
)

var (
	ErrInvalidOp        = errors.New("некорректная операция над таблицей")
	ErrWorkbookTooLarge = errors.New("размер книги превышает лимит")
)

const (
	maxOpsPerRequest      = 500
	maxRowsAppend         = 10000
	maxColsPerRow         = 512
	maxSheetNameLen       = 31
	maxPreviewRows        = 500
	maxPreviewCols        = 64
	maxSheetsInWorkbook   = 100
	maxImportCSVTextBytes = 512 * 1024
	maxExportCSVBytes     = 512 * 1024
)

func Apply(workbook []byte, operationsJSON string, previewSheet, previewRange string) (out []byte, previewTSV string, exportedCSV string, err error) {
	if len(workbook) > document.MaxRecommendedAttachmentSizeBytes {
		return nil, "", "", ErrWorkbookTooLarge
	}

	ops, err := parseOps(operationsJSON)
	if err != nil {
		return nil, "", "", err
	}
	if len(ops) > maxOpsPerRequest {
		return nil, "", "", fmt.Errorf("%w: слишком много операций (%d)", ErrInvalidOp, len(ops))
	}

	var f *excelize.File
	if len(workbook) == 0 {
		f = excelize.NewFile()
	} else {
		f, err = excelize.OpenReader(bytes.NewReader(workbook))
		if err != nil {
			return nil, "", "", fmt.Errorf("открытие xlsx: %w", err)
		}
	}

	defer func() { _ = f.Close() }()

	if n := len(f.GetSheetList()); n > maxSheetsInWorkbook {
		return nil, "", "", fmt.Errorf("%w: в книге слишком много листов (%d)", ErrInvalidOp, n)
	}

	for i, op := range ops {
		if exp, ok := op.(opExportSheetCSV); ok {
			s, err := exportSheetToCSV(f, exp.Sheet, commaRuneFromOp(exp.Comma))
			if err != nil {
				return nil, "", "", fmt.Errorf("операция #%d (%s): %w", i+1, op.kind(), err)
			}

			exportedCSV = s
			continue
		}

		if err := execOp(f, op); err != nil {
			return nil, "", "", fmt.Errorf("операция #%d (%s): %w", i+1, op.kind(), err)
		}

		if len(f.GetSheetList()) > maxSheetsInWorkbook {
			return nil, "", "", fmt.Errorf("%w: слишком много листов в книге", ErrInvalidOp)
		}
	}

	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, "", "", fmt.Errorf("запись xlsx: %w", err)
	}

	out = buf.Bytes()
	if len(out) > document.MaxRecommendedAttachmentSizeBytes {
		return nil, "", "", ErrWorkbookTooLarge
	}

	ps := strings.TrimSpace(previewSheet)
	pr := strings.TrimSpace(previewRange)
	if ps != "" {
		previewTSV, err = exportRangeTSV(f, ps, pr)
		if err != nil {
			return nil, "", "", fmt.Errorf("предпросмотр: %w", err)
		}
	}

	return out, previewTSV, exportedCSV, nil
}

type op interface {
	kind() string
}

type opEnsureSheet struct {
	Op   string `json:"op"`
	Name string `json:"name"`
}

func (o opEnsureSheet) kind() string { return "ensure_sheet" }

type opRenameSheet struct {
	Op  string `json:"op"`
	Old string `json:"old"`
	New string `json:"new"`
}

func (o opRenameSheet) kind() string { return "rename_sheet" }

type opSetCell struct {
	Op    string `json:"op"`
	Sheet string `json:"sheet"`
	Cell  string `json:"cell"`
	Value string `json:"value"`
}

func (o opSetCell) kind() string { return "set_cell" }

type opAppendRows struct {
	Op    string     `json:"op"`
	Sheet string     `json:"sheet"`
	Rows  [][]string `json:"rows"`
}

func (o opAppendRows) kind() string { return "append_rows" }

type opImportCSVText struct {
	Op    string `json:"op"`
	Sheet string `json:"sheet"`
	CSV   string `json:"csv"`
	Comma string `json:"comma,omitempty"`
}

func (o opImportCSVText) kind() string { return "import_csv_text" }

type opExportSheetCSV struct {
	Op    string `json:"op"`
	Sheet string `json:"sheet"`
	Comma string `json:"comma,omitempty"`
}

func (o opExportSheetCSV) kind() string { return "export_sheet_csv" }

func parseOps(raw string) ([]op, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	var messages []json.RawMessage
	if err := json.Unmarshal([]byte(raw), &messages); err != nil {
		return nil, fmt.Errorf("%w: ожидается JSON-массив операций: %v", ErrInvalidOp, err)
	}

	out := make([]op, 0, len(messages))
	for _, m := range messages {
		var head struct {
			Op string `json:"op"`
		}

		if err := json.Unmarshal(m, &head); err != nil {
			return nil, err
		}

		switch strings.TrimSpace(strings.ToLower(head.Op)) {
		case "ensure_sheet":
			var o opEnsureSheet
			if err := json.Unmarshal(m, &o); err != nil {
				return nil, err
			}

			if err := validateSheetName(o.Name); err != nil {
				return nil, err
			}
			out = append(out, o)
		case "rename_sheet":
			var o opRenameSheet
			if err := json.Unmarshal(m, &o); err != nil {
				return nil, err
			}

			if err := validateSheetName(o.Old); err != nil {
				return nil, err
			}

			if err := validateSheetName(o.New); err != nil {
				return nil, err
			}

			out = append(out, o)
		case "set_cell":
			var o opSetCell
			if err := json.Unmarshal(m, &o); err != nil {
				return nil, err
			}

			if err := validateSheetName(o.Sheet); err != nil {
				return nil, err
			}

			o.Cell = strings.TrimSpace(o.Cell)
			if o.Cell == "" {
				return nil, fmt.Errorf("%w: пустая ячейка", ErrInvalidOp)
			}

			if _, _, err := excelize.CellNameToCoordinates(o.Cell); err != nil {
				return nil, fmt.Errorf("%w: адрес ячейки %q: %v", ErrInvalidOp, o.Cell, err)
			}
			out = append(out, o)
		case "import_csv_text":
			var o opImportCSVText
			if err := json.Unmarshal(m, &o); err != nil {
				return nil, err
			}

			if err := validateSheetName(o.Sheet); err != nil {
				return nil, err
			}

			if strings.TrimSpace(o.CSV) == "" {
				return nil, fmt.Errorf("%w: пустое поле csv", ErrInvalidOp)
			}

			if len(o.CSV) > maxImportCSVTextBytes {
				return nil, fmt.Errorf("%w: csv длиннее %d байт", ErrInvalidOp, maxImportCSVTextBytes)
			}

			if !utf8.ValidString(o.CSV) {
				return nil, fmt.Errorf("%w: csv должен быть в UTF-8", ErrInvalidOp)
			}

			comma := strings.TrimSpace(o.Comma)
			if comma != "" && utf8.RuneCountInString(comma) != 1 {
				return nil, fmt.Errorf("%w: comma должен быть одним символом", ErrInvalidOp)
			}

			out = append(out, o)
		case "append_rows":
			var o opAppendRows
			if err := json.Unmarshal(m, &o); err != nil {
				return nil, err
			}

			if err := validateSheetName(o.Sheet); err != nil {
				return nil, err
			}

			if len(o.Rows) > maxRowsAppend {
				return nil, fmt.Errorf("%w: append_rows больше %d строк", ErrInvalidOp, maxRowsAppend)
			}

			for _, row := range o.Rows {
				if len(row) > maxColsPerRow {
					return nil, fmt.Errorf("%w: в строке больше %d столбцов", ErrInvalidOp, maxColsPerRow)
				}
			}
			out = append(out, o)
		case "export_sheet_csv":
			var o opExportSheetCSV
			if err := json.Unmarshal(m, &o); err != nil {
				return nil, err
			}

			if err := validateSheetName(o.Sheet); err != nil {
				return nil, err
			}

			comma := strings.TrimSpace(o.Comma)
			if comma != "" && utf8.RuneCountInString(comma) != 1 {
				return nil, fmt.Errorf("%w: comma должен быть одним символом", ErrInvalidOp)
			}

			out = append(out, o)
		default:
			if strings.TrimSpace(head.Op) == "" {
				return nil, fmt.Errorf("%w: отсутствует поле op", ErrInvalidOp)
			}

			return nil, fmt.Errorf("%w: неизвестная операция %q", ErrInvalidOp, head.Op)
		}
	}
	return out, nil
}

func validateSheetName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("%w: пустое имя листа", ErrInvalidOp)
	}

	if len(name) > maxSheetNameLen {
		return fmt.Errorf("%w: имя листа длиннее %d", ErrInvalidOp, maxSheetNameLen)
	}

	for _, r := range name {
		switch r {
		case '\\', '/', '?', '*', '[', ']':
			return fmt.Errorf("%w: недопустимый символ в имени листа", ErrInvalidOp)
		}
	}
	return nil
}

func execOp(f *excelize.File, o op) error {
	switch t := o.(type) {
	case opEnsureSheet:
		idx, err := f.GetSheetIndex(t.Name)
		if err != nil {
			return err
		}

		if idx < 0 {
			_, err = f.NewSheet(t.Name)
			return err
		}
		return nil
	case opRenameSheet:
		return f.SetSheetName(t.Old, t.New)
	case opSetCell:
		return f.SetCellValue(t.Sheet, t.Cell, t.Value)
	case opAppendRows:
		existing, err := f.GetRows(t.Sheet)
		if err != nil {
			return err
		}

		startRow := max(len(existing)+1, 1)

		for ri, row := range t.Rows {
			for ci, val := range row {
				col := ci + 1
				rowIdx := startRow + ri
				cell, err := excelize.CoordinatesToCellName(col, rowIdx)
				if err != nil {
					return err
				}

				if err := f.SetCellValue(t.Sheet, cell, val); err != nil {
					return err
				}
			}
		}
		return nil
	case opImportCSVText:
		idx, err := f.GetSheetIndex(t.Sheet)
		if err != nil {
			return err
		}

		if idx < 0 {
			if _, err := f.NewSheet(t.Sheet); err != nil {
				return err
			}
		}

		records, err := parseCSVRecords(t.CSV, commaRuneFromOp(t.Comma))
		if err != nil {
			return err
		}

		if len(records) > maxRowsAppend {
			return fmt.Errorf("%w: в csv больше %d строк", ErrInvalidOp, maxRowsAppend)
		}

		for ri, row := range records {
			if len(row) > maxColsPerRow {
				return fmt.Errorf("%w: в строке csv больше %d столбцов", ErrInvalidOp, maxColsPerRow)
			}

			for ci, val := range row {
				cell, err := excelize.CoordinatesToCellName(ci+1, ri+1)
				if err != nil {
					return err
				}

				if err := f.SetCellValue(t.Sheet, cell, val); err != nil {
					return err
				}
			}
		}
		return nil
	default:
		return ErrInvalidOp
	}
}

func commaRuneFromOp(comma string) rune {
	comma = strings.TrimSpace(comma)
	if comma == "" {
		return 0
	}

	r, _ := utf8.DecodeRuneInString(comma)

	return r
}

func parseCSVRecords(text string, comma rune) ([][]string, error) {
	r := csv.NewReader(strings.NewReader(text))
	r.LazyQuotes = true
	if comma != 0 {
		r.Comma = comma
	} else {
		r.Comma = detectCSVComma(text)
	}

	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("разбор CSV: %w", err)
	}

	return records, nil
}

func detectCSVComma(s string) rune {
	firstLine := s
	if before, _, ok := strings.Cut(s, "\n"); ok {
		firstLine = before
	}

	if strings.Contains(firstLine, ";") && !strings.Contains(firstLine, ",") {
		return ';'
	}

	return ','
}

func exportSheetToCSV(f *excelize.File, sheet string, comma rune) (string, error) {
	idx, err := f.GetSheetIndex(sheet)
	if err != nil {
		return "", err
	}

	if idx < 0 {
		return "", fmt.Errorf("лист %q не найден", sheet)
	}

	if comma == 0 {
		comma = ','
	}

	rows, err := f.GetRows(sheet)
	if err != nil {
		return "", err
	}

	if len(rows) > maxRowsAppend {
		return "", fmt.Errorf("лист %q: слишком много строк для экспорта (%d > %d)", sheet, len(rows), maxRowsAppend)
	}

	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Comma = comma
	for _, row := range rows {
		if len(row) > maxColsPerRow {
			return "", fmt.Errorf("лист %q: в строке больше %d столбцов", sheet, maxColsPerRow)
		}

		if err := w.Write(row); err != nil {
			return "", err
		}
	}

	w.Flush()
	if err := w.Error(); err != nil {
		return "", err
	}

	s := buf.String()
	if len(s) > maxExportCSVBytes {
		return "", fmt.Errorf("экспорт CSV превышает %d байт", maxExportCSVBytes)
	}

	return s, nil
}

func exportRangeTSV(f *excelize.File, sheet, cellRange string) (string, error) {
	if err := validateSheetName(sheet); err != nil {
		return "", err
	}

	idx, err := f.GetSheetIndex(sheet)
	if err != nil {
		return "", err
	}

	if idx == -1 {
		return "", fmt.Errorf("лист %q не найден", sheet)
	}

	var minRow, maxRow, minCol, maxCol int
	if strings.TrimSpace(cellRange) == "" {
		rows, err := f.GetRows(sheet)
		if err != nil {
			return "", err
		}

		minRow, minCol = 1, 1
		maxRow = len(rows)
		if maxRow == 0 {
			return "", nil
		}

		maxCol = 0
		for _, r := range rows {
			if len(r) > maxCol {
				maxCol = len(r)
			}
		}

		if maxRow > maxPreviewRows {
			maxRow = maxPreviewRows
		}

		if maxCol > maxPreviewCols {
			maxCol = maxPreviewCols
		}
	} else {
		parts := strings.Split(strings.TrimSpace(cellRange), ":")
		if len(parts) != 2 {
			return "", fmt.Errorf("диапазон ожидается как A1:B10, получено %q", cellRange)
		}

		c1, r1, err := excelize.CellNameToCoordinates(strings.TrimSpace(parts[0]))
		if err != nil {
			return "", err
		}

		c2, r2, err := excelize.CellNameToCoordinates(strings.TrimSpace(parts[1]))
		if err != nil {
			return "", err
		}

		minCol, maxCol = c1, c2
		minRow, maxRow = r1, r2
		if minCol > maxCol {
			minCol, maxCol = maxCol, minCol
		}

		if minRow > maxRow {
			minRow, maxRow = maxRow, minRow
		}

		if maxRow-minRow+1 > maxPreviewRows {
			maxRow = minRow + maxPreviewRows - 1
		}

		if maxCol-minCol+1 > maxPreviewCols {
			maxCol = minCol + maxPreviewCols - 1
		}
	}

	var b strings.Builder
	for r := minRow; r <= maxRow; r++ {
		for c := minCol; c <= maxCol; c++ {
			if c > minCol {
				b.WriteByte('\t')
			}

			cell, err := excelize.CoordinatesToCellName(c, r)
			if err != nil {
				return "", err
			}

			v, err := f.GetCellValue(sheet, cell)
			if err != nil {
				return "", err
			}

			b.WriteString(v)
		}
		b.WriteByte('\n')
	}

	return strings.TrimSuffix(b.String(), "\n"), nil
}
