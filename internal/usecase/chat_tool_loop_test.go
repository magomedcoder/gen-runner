package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExtractLeadingJSONArray(t *testing.T) {
	raw := `[{"tool_name":"build_docx","parameters":{"spec_json":"{}"}}]`
	if got := extractLeadingJSONArray("  \n" + raw + "\n"); got != raw {
		t.Fatalf("extractLeadingJSONArray: %q", got)
	}
	withNoise := `[{"note":"edge: ] and [ in string","tool_name":"x","parameters":{}}]`
	if got := extractLeadingJSONArray(withNoise); got != withNoise {
		t.Fatalf("внутри строки скобки: %q", got)
	}
}

func TestExtractToolActionBlob_rawPrefix(t *testing.T) {
	blob := extractToolActionBlob(`[{"tool_name":"apply_spreadsheet","parameters":{"operations_json":"[]"}}]`)
	rows, err := parseCohereActionList(blob)
	if err != nil || len(rows) != 1 || rows[0].ToolName != "apply_spreadsheet" {
		t.Fatalf("blob=%q err=%v rows=%v", blob, err, rows)
	}
}

func TestExtractToolActionBlob_embeddedAfterPreamble(t *testing.T) {
	text := "Кратко: обновлю книгу.\n\n" +
		`[{"tool_name":"apply_spreadsheet","parameters":{"operations_json":"[]"}}]` +
		"\n\nГотово."
	blob := extractToolActionBlob(text)
	rows, err := parseCohereActionList(blob)
	if err != nil || len(rows) != 1 || rows[0].ToolName != "apply_spreadsheet" {
		t.Fatalf("blob=%q err=%v rows=%v", blob, err, rows)
	}
}

func TestExtractToolActionBlob_genericCodeFence(t *testing.T) {
	text := "Вот вызов:\n\n```\n" +
		`[{"tool_name":"build_docx","parameters":{"spec_json":"{}"}}]` +
		"\n```\n"
	blob := extractToolActionBlob(text)
	rows, err := parseCohereActionList(blob)
	if err != nil || len(rows) != 1 || rows[0].ToolName != "build_docx" {
		t.Fatalf("blob=%q err=%v rows=%v", blob, err, rows)
	}
}

func TestExtractCohereActionJSON(t *testing.T) {
	text := `Краткое рассуждение здесь.

Действие: ` + "```json\n[\n  {\"tool_name\": \"apply_spreadsheet\", \"parameters\": {\"operations_json\": \"[]\"}}\n]\n```"

	got := extractCohereActionJSON(text)
	if !strings.Contains(got, "apply_spreadsheet") {
		t.Fatalf("ожидался JSON с apply_spreadsheet, получено: %q", got)
	}
	rows, err := parseCohereActionList(got)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ToolName != "apply_spreadsheet" {
		t.Fatalf("неверный разбор: %+v", rows)
	}
}

func TestFilterExecutableToolRows(t *testing.T) {
	rows := []cohereActionRow{
		{ToolName: "directly-answer", Parameters: []byte(`{"answer":"привет"}`)},
		{ToolName: "apply_spreadsheet", Parameters: []byte(`{"operations_json":"[]"}`)},
	}
	out := filterExecutableToolRows(rows)
	if len(out) != 1 || out[0].ToolName != "apply_spreadsheet" {
		t.Fatalf("ожидалась одна строка apply_spreadsheet, получено %+v", out)
	}
}

func TestToolExecutionDuration(t *testing.T) {
	if d := toolExecutionDuration(0); d != defaultToolExecSeconds*time.Second {
		t.Fatalf("0 → %v, ожидалось %v", d, defaultToolExecSeconds*time.Second)
	}
	if d := toolExecutionDuration(10); d != minToolExecSeconds*time.Second {
		t.Fatalf("10 → %v, ожидалось %v", d, minToolExecSeconds*time.Second)
	}
	if d := toolExecutionDuration(600); d != maxToolExecSeconds*time.Second {
		t.Fatalf("600 → %v, ожидалось %v", d, maxToolExecSeconds*time.Second)
	}
	if d := toolExecutionDuration(90); d != 90*time.Second {
		t.Fatalf("90 → %v", d)
	}
}

func TestRunFnWithContextNoDeadline(t *testing.T) {
	v, err := runFnWithContext(context.Background(), func() (int, error) {
		return 42, nil
	})
	if err != nil || v != 42 {
		t.Fatalf("got %v, %v", v, err)
	}
}

func TestRunFnWithContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	_, err := runFnWithContext(ctx, func() (int, error) {
		time.Sleep(50 * time.Millisecond)
		return 1, nil
	})
	if err == nil || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ожидался DeadlineExceeded, err=%v", err)
	}
}
