package ovf

import (
	"bufio"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/mitchellh/packer/common"
	"github.com/mitchellh/packer/template/interpolate"
)

// Config is the configuration structure for the builder.
type OVFConfig struct {
	SourcePath           string   `mapstructure:"source_path"`
	Checksum             string   `mapstructure:"checksum"`
	ChecksumURL          string   `mapstructure:"checksum_url"`
	ChecksumType         string   `mapstructure:"checksum_type"`
	TargetPath           string   `mapstructure:"target_path"`
}

func (c * OVFConfig) Prepare(ctx *interpolate.Context) ([]string, []error) {
	// Validation
	var errs []error
	var err error
	var warnings []string

	if c.SourcePath == "" {
		errs = append(errs, errors.New("The source_path must be specified."))
	}

	if c.ChecksumType == "" {
		errs = append(errs, errors.New("The checksum_type must be specified."))
	} else {
		c.ChecksumType = strings.ToLower(c.ChecksumType)
		if c.ChecksumType != "none" {
			if c.Checksum == "" && c.ChecksumURL == "" {
				errs = append(errs, errors.New("Due to large file sizes, an checksum is required"))
				return warnings, errs
			} else {
				if h := common.HashForType(c.ChecksumType); h == nil {
					errs = append(errs, fmt.Errorf("Unsupported checksum type: %s", c.ChecksumType))
					return warnings, errs
				}

				// If checksum has no value, use checksum_url instead.
				if c.Checksum == "" {
					u, err := url.Parse(c.ChecksumURL)
					if err != nil {
						errs = append(errs, fmt.Errorf("Error parsing checksum: %s", err))
						return warnings, errs
					}
					switch u.Scheme {
					case "http", "https":
						res, err := http.Get(c.ChecksumURL)
						c.Checksum = ""
						if err != nil {
							errs = append(errs, fmt.Errorf("Error getting checksum from url: %s", c.ChecksumURL))
							return warnings, errs
						}
						defer res.Body.Close()
						err = c.parseCheckSumFile(bufio.NewReader(res.Body))
						if err != nil {
							errs = append(errs, err)
							return warnings, errs
						}
					case "file":
						file, err := os.Open(u.Path)
						if err != nil {
							errs = append(errs, err)
							return warnings, errs
						}
						err = c.parseCheckSumFile(bufio.NewReader(file))
						if err != nil {
							errs = append(errs, err)
							return warnings, errs
						}
					case "":
						break
					default:
						errs = append(errs, fmt.Errorf("Error parsing checksum url: %s, scheme not supported: %s", c.ChecksumURL, u.Scheme))
						return warnings, errs
					}
				}
			}
		}
	}

	c.Checksum = strings.ToLower(c.Checksum)
	c.SourcePath, err = common.DownloadableURL(c.SourcePath)
	if err != nil {
		errs = append(errs, fmt.Errorf("Failed to parse source_path: %s", err))
	}

	// Warnings
	if c.ChecksumType == "none" {
		warnings = append(warnings, "A checksum type of 'none' was specified. Since OVA files can be big,\n"+
			"a checksum is highly recommended.")
	}

	return warnings, errs
}

func (c *OVFConfig) parseCheckSumFile(rd *bufio.Reader) error {
	errNotFound := fmt.Errorf("No checksum for %q found at: %s", filepath.Base(c.SourcePath), c.ChecksumURL)
	for {
		line, err := rd.ReadString('\n')
		if err != nil && line == "" {
			break
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		if strings.ToLower(parts[0]) == c.ChecksumType {
			// BSD-style checksum
			if parts[1] == fmt.Sprintf("(%s)", filepath.Base(c.SourcePath)) {
				c.Checksum = parts[3]
				return nil
			}
		} else {
			// Standard checksum
			if parts[1][0] == '*' {
				// Binary mode
				parts[1] = parts[1][1:]
			}
			if parts[1] == filepath.Base(c.SourcePath) {
				c.Checksum = parts[0]
				return nil
			}
		}
	}
	return errNotFound
}
