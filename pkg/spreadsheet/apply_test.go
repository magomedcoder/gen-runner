package spreadsheet

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestApplyNewWorkbookSetCellAndPreview(t *testing.T) {
	ops := `[
		{"op":"ensure_sheet","name":"Data"},
		{"op":"set_cell","sheet":"Data","cell":"A1","value":"строка1"},
		{"op":"set_cell","sheet":"Data","cell":"B2","value":"строка2"}
	]`
	out, preview, _, err := Apply(nil, ops, "Data", "A1:B2")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) < 100 {
		t.Fatalf("xlsx too small: %d", len(out))
	}
	if !strings.Contains(preview, "строка1") || !strings.Contains(preview, "строка2") {
		t.Fatalf("preview: %q", preview)
	}
}

func TestApplyImportCSVText(t *testing.T) {
	csv := "имя;значение\nодин;1\nдва;2"
	ops := fmt.Sprintf(`[
		{"op":"import_csv_text","sheet":"Csv","csv":%q}
	]`, csv)
	out, _, _, err := Apply(nil, ops, "Csv", "A1:B3")
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelizeOpenBytes(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	v, err := f.GetCellValue("Csv", "A1")
	if err != nil || v != "имя" {
		t.Fatalf("A1=%q err=%v", v, err)
	}
	v, err = f.GetCellValue("Csv", "B2")
	if err != nil || v != "1" {
		t.Fatalf("B2=%q err=%v", v, err)
	}
}

func TestApplyExportSheetCSV(t *testing.T) {
	ops := `[
		{"op":"ensure_sheet","name":"S"},
		{"op":"set_cell","sheet":"S","cell":"A1","value":"левый"},
		{"op":"set_cell","sheet":"S","cell":"B1","value":"правый"},
		{"op":"export_sheet_csv","sheet":"S"}
	]`
	_, _, csv, err := Apply(nil, ops, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if csv != "левый,правый\n" {
		t.Fatalf("csv: %q", csv)
	}
}

func TestApplyAppendRows(t *testing.T) {
	ops := `[
		{"op":"ensure_sheet","name":"S"},
		{"op":"append_rows","sheet":"S","rows":[["ячейка1","ячейка2"],["ячейка3","ячейка4"]]}
	]`
	out, _, _, err := Apply(nil, ops, "", "")
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelizeOpenBytes(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	v, err := f.GetCellValue("S", "A1")
	if err != nil || v != "ячейка1" {
		t.Fatalf("A1=%q err=%v", v, err)
	}
	v, err = f.GetCellValue("S", "B2")
	if err != nil || v != "ячейка4" {
		t.Fatalf("B2=%q err=%v", v, err)
	}
}

func excelizeOpenBytes(b []byte) (*excelize.File, error) {
	return excelize.OpenReader(bytes.NewReader(b))
}
