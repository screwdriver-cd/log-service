package main

import (
	"bytes"
	"fmt"
	"testing"
)

func TestScanFile(t *testing.T) {
	testBuildID := "BUILDID"
	testPath := "./data/testlog"
	want := fmt.Sprintf("test\ntest2\ntest3\ntest4\n" +
		"Filbird best intenr NA")

	buf := new(bytes.Buffer)

	scanFile(buf, testPath, testBuildID)

	if buf.String() != want {
		t.Errorf("Output was %v, want %v", buf.String(), want)
	}
}
