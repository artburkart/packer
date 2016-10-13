package ovf

import (
	"fmt"
	"io/ioutil"
	"testing"
)

func testOVFConfig() OVFConfig {
	return OVFConfig{
		Checksum:     "foo",
		ChecksumURL:  "",
		ChecksumType: "md5",
		SourcePath: "http://www.packer.io/the-OS.ova",
	}
}

var cs_bsd_style = `
MD5 (other.ova) = bAr
MD5 (the-OS.ova) = baZ
`

var cs_gnu_style = `
bAr0 *the-OS.ova
baZ0  other.ova
`

var cs_bsd_style_no_newline = `
MD5 (other.ova) = bAr
MD5 (the-OS.ova) = baZ`

var cs_gnu_style_no_newline = `
bAr0 *the-OS.ova
baZ0  other.ova`

func TestOVFConfigPrepare_Checksum(t *testing.T) {
	i := testOVFConfig()

	// Test bad
	i.Checksum = ""
	warns, err := i.Prepare(nil)
	if len(warns) > 0 {
		t.Fatalf("bad: %#v", warns)
	}
	if err == nil {
		t.Fatal("should have error")
	}

	// Test good
	i = testOVFConfig()
	i.Checksum = "FOo"
	warns, err = i.Prepare(nil)
	if len(warns) > 0 {
		t.Fatalf("bad: %#v", warns)
	}
	if err != nil {
		t.Fatalf("should not have error: %s", err)
	}

	if i.Checksum != "foo" {
		t.Fatalf("should've lowercased: %s", i.Checksum)
	}
}

func TestOVFConfigPrepare_ChecksumURL(t *testing.T) {
	i := testOVFConfig()
	i.ChecksumURL = "file:///not_read"

	// Test Checksum overrides url
	warns, err := i.Prepare(nil)
	if len(warns) > 0 && len(err) > 0 {
		t.Fatalf("bad: %#v, %#v", warns, err)
	}

	// Test good - ChecksumURL BSD style
	i = testOVFConfig()
	i.Checksum = ""
	cs_file, _ := ioutil.TempFile("", "packer-test-")
	ioutil.WriteFile(cs_file.Name(), []byte(cs_bsd_style), 0666)
	i.ChecksumURL = fmt.Sprintf("file://%s", cs_file.Name())
	warns, err = i.Prepare(nil)
	if len(warns) > 0 {
		t.Fatalf("bad: %#v", warns)
	}
	if err != nil {
		t.Fatalf("should not have error: %s", err)
	}

	if i.Checksum != "baz" {
		t.Fatalf("should've found \"baz\" got: %s", i.Checksum)
	}

	// Test good - ChecksumURL GNU style
	i = testOVFConfig()
	i.Checksum = ""
	cs_file, _ = ioutil.TempFile("", "packer-test-")
	ioutil.WriteFile(cs_file.Name(), []byte(cs_gnu_style), 0666)
	i.ChecksumURL = fmt.Sprintf("file://%s", cs_file.Name())
	warns, err = i.Prepare(nil)
	if len(warns) > 0 {
		t.Fatalf("bad: %#v", warns)
	}
	if err != nil {
		t.Fatalf("should not have error: %s", err)
	}

	if i.Checksum != "bar0" {
		t.Fatalf("should've found \"bar0\" got: %s", i.Checksum)
	}

	// Test good - ChecksumURL BSD style no newline
	i = testOVFConfig()
	i.Checksum = ""
	cs_file, _ = ioutil.TempFile("", "packer-test-")
	ioutil.WriteFile(cs_file.Name(), []byte(cs_bsd_style_no_newline), 0666)
	i.ChecksumURL = fmt.Sprintf("file://%s", cs_file.Name())
	warns, err = i.Prepare(nil)
	if len(warns) > 0 {
		t.Fatalf("bad: %#v", warns)
	}
	if err != nil {
		t.Fatalf("should not have error: %s", err)
	}

	if i.Checksum != "baz" {
		t.Fatalf("should've found \"baz\" got: %s", i.Checksum)
	}

	// Test good - ChecksumURL GNU style no newline
	i = testOVFConfig()
	i.Checksum = ""
	cs_file, _ = ioutil.TempFile("", "packer-test-")
	ioutil.WriteFile(cs_file.Name(), []byte(cs_gnu_style_no_newline), 0666)
	i.ChecksumURL = fmt.Sprintf("file://%s", cs_file.Name())
	warns, err = i.Prepare(nil)
	if len(warns) > 0 {
		t.Fatalf("bad: %#v", warns)
	}
	if err != nil {
		t.Fatalf("should not have error: %s", err)
	}

	if i.Checksum != "bar0" {
		t.Fatalf("should've found \"bar0\" got: %s", i.Checksum)
	}
}

func TestOVFConfigPrepare_ChecksumType(t *testing.T) {
	i := testOVFConfig()

	// Test bad
	i.ChecksumType = ""
	warns, err := i.Prepare(nil)
	if len(warns) > 0 {
		t.Fatalf("bad: %#v", warns)
	}
	if err == nil {
		t.Fatal("should have error")
	}

	// Test good
	i = testOVFConfig()
	i.ChecksumType = "mD5"
	warns, err = i.Prepare(nil)
	if len(warns) > 0 {
		t.Fatalf("bad: %#v", warns)
	}
	if err != nil {
		t.Fatalf("should not have error: %s", err)
	}

	if i.ChecksumType != "md5" {
		t.Fatalf("should've lowercased: %s", i.ChecksumType)
	}

	// Test unknown
	i = testOVFConfig()
	i.ChecksumType = "fake"
	warns, err = i.Prepare(nil)
	if len(warns) > 0 {
		t.Fatalf("bad: %#v", warns)
	}
	if err == nil {
		t.Fatal("should have error")
	}

	// Test none
	i = testOVFConfig()
	i.ChecksumType = "none"
	warns, err = i.Prepare(nil)
	if len(warns) == 0 {
		t.Fatalf("bad: %#v", warns)
	}
	if err != nil {
		t.Fatalf("should not have error: %s", err)
	}

	if i.ChecksumType != "none" {
		t.Fatalf("should've lowercased: %s", i.ChecksumType)
	}
}

func TestOVFConfigPrepare_OVAUrl(t *testing.T) {
	i := testOVFConfig()

	// Test source_path empty
	i.SourcePath = ""
	warns, err := i.Prepare(nil)
	if len(warns) > 0 {
		t.Fatalf("bad: %#v", warns)
	}
	if err == nil {
		t.Fatal("should have error")
	}

	// Test source_path set as url
	i = testOVFConfig()
	i.SourcePath = "http://www.packer.io/the-OS.iso"
	warns, err = i.Prepare(nil)
	if len(warns) > 0 {
		t.Fatalf("bad: %#v", warns)
	}
	if err != nil {
		t.Errorf("should not have error: %s", err)
	}

	// Test source_path set as filepath
	i = testOVFConfig()
	i.SourcePath = "/i/dont/exist"
	warns, err = i.Prepare(nil)
	if len(warns) > 0 {
		t.Fatalf("bad: %#v", warns)
	}
	if err != nil {
		t.Errorf("should not have error: %s", err)
	}
}
