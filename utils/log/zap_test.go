package log

import (
	"bufio"
	"context"
	"encoding/json"
	"go.uber.org/zap"
	"os"
	"testing"
)

func TestInfo(t *testing.T) {
	type testData struct {
		msg    string
		fields []zap.Field
	}

	ctx := context.Background()
	tests := []testData{
		{
			msg: "test",
		},
		{
			msg:    "test",
			fields: []zap.Field{zap.String("key", "value")},
		},
		{
			msg:    "test",
			fields: []zap.Field{zap.String("key", "value"), zap.Int("intKey", 1)},
		},
		{
			msg:    "test",
			fields: []zap.Field{zap.String("key", "value"), zap.Int("intKey", 1), zap.Float64("floatKey", 1.1)},
		},
	}

	for _, test := range tests {
		Info(ctx, test.msg, test.fields...)
	}

	// read log file and check
	logFile, err := os.Open(logFilePath)
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()

	scanner := bufio.NewScanner(logFile)
	for _, test := range tests {
		if !scanner.Scan() {
			t.Fatal("log file is empty")
		}

		line := scanner.Text()
		var m map[string]any
		err = json.Unmarshal([]byte(line), &m)
		if err != nil {
			t.Fatal(err)
		}

		if m["msg"] != test.msg {
			t.Errorf("expect %s, got %s", test.msg, m["msg"])
		}
	}

	// delete log file
	err = os.Remove(logFilePath)
	if err != nil {
		t.Fatal(err)
	}
}
