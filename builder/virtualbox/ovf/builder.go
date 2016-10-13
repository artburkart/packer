package ovf

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/mitchellh/multistep"
	vboxcommon "github.com/mitchellh/packer/builder/virtualbox/common"
	"github.com/mitchellh/packer/common"
	"github.com/mitchellh/packer/helper/communicator"
	"github.com/mitchellh/packer/helper/config"
	"github.com/mitchellh/packer/packer"
	"github.com/mitchellh/packer/template/interpolate"
)

// Builder implements packer.Builder and builds the actual VirtualBox
// images.
type Builder struct {
	config Config
	runner multistep.Runner
}

type Config struct {
	common.PackerConfig             `mapstructure:",squash"`
	common.HTTPConfig               `mapstructure:",squash"`
	OVFConfig                       `mapstructure:",squash"`
	common.FloppyConfig             `mapstructure:",squash"`
	vboxcommon.ExportConfig         `mapstructure:",squash"`
	vboxcommon.ExportOpts           `mapstructure:",squash"`
	vboxcommon.OutputConfig         `mapstructure:",squash"`
	vboxcommon.RunConfig            `mapstructure:",squash"`
	vboxcommon.SSHConfig            `mapstructure:",squash"`
	vboxcommon.ShutdownConfig       `mapstructure:",squash"`
	vboxcommon.VBoxManageConfig     `mapstructure:",squash"`
	vboxcommon.VBoxManagePostConfig `mapstructure:",squash"`
	vboxcommon.VBoxVersionConfig    `mapstructure:",squash"`

	BootCommand          []string `mapstructure:"boot_command"`
	GuestAdditionsMode   string   `mapstructure:"guest_additions_mode"`
	GuestAdditionsPath   string   `mapstructure:"guest_additions_path"`
	GuestAdditionsURL    string   `mapstructure:"guest_additions_url"`
	GuestAdditionsSHA256 string   `mapstructure:"guest_additions_sha256"`
	ImportOpts           string   `mapstructure:"import_opts"`
	ImportFlags          []string `mapstructure:"import_flags"`
	VMName               string   `mapstructure:"vm_name"`

	ctx interpolate.Context
}

// Prepare processes the build configuration parameters.
func (b *Builder) Prepare(raws ...interface{}) ([]string, error) {
	err := config.Decode(&b.config, &config.DecodeOpts{
		Interpolate:        true,
		InterpolateContext: &b.config.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{
				"boot_command",
				"guest_additions_path",
				"guest_additions_url",
				"vboxmanage",
				"vboxmanage_post",
			},
		},
	}, raws...)
	if err != nil {
		return nil, err
	}

	// Accumulate any errors and warnings
	var errs *packer.MultiError
	warnings := make([]string, 0)

	ovfWarnings, ovfErrs := b.config.OVFConfig.Prepare(&b.config.ctx)
	warnings = append(warnings, ovfWarnings...)
	errs = packer.MultiErrorAppend(errs, ovfErrs...)

	errs = packer.MultiErrorAppend(errs, b.config.ExportConfig.Prepare(&b.config.ctx)...)
	errs = packer.MultiErrorAppend(errs, b.config.ExportOpts.Prepare(&b.config.ctx)...)
	errs = packer.MultiErrorAppend(errs, b.config.FloppyConfig.Prepare(&b.config.ctx)...)
	errs = packer.MultiErrorAppend(errs, b.config.HTTPConfig.Prepare(&b.config.ctx)...)
	errs = packer.MultiErrorAppend(errs, b.config.OutputConfig.Prepare(&b.config.ctx, &b.config.PackerConfig)...)
	errs = packer.MultiErrorAppend(errs, b.config.RunConfig.Prepare(&b.config.ctx)...)
	errs = packer.MultiErrorAppend(errs, b.config.ShutdownConfig.Prepare(&b.config.ctx)...)
	errs = packer.MultiErrorAppend(errs, b.config.SSHConfig.Prepare(&b.config.ctx)...)
	errs = packer.MultiErrorAppend(errs, b.config.VBoxManageConfig.Prepare(&b.config.ctx)...)
	errs = packer.MultiErrorAppend(errs, b.config.VBoxManagePostConfig.Prepare(&b.config.ctx)...)
	errs = packer.MultiErrorAppend(errs, b.config.VBoxVersionConfig.Prepare(&b.config.ctx)...)

	if b.config.GuestAdditionsMode == "" {
		b.config.GuestAdditionsMode = "upload"
	}

	if b.config.GuestAdditionsPath == "" {
		b.config.GuestAdditionsPath = "VBoxGuestAdditions.iso"
	}

	if b.config.VMName == "" {
		b.config.VMName = fmt.Sprintf("packer-%s-%d", b.config.PackerBuildName, interpolate.InitTime.Unix())
	}

	// TODO: Write a packer fix and just remove import_opts
	// TODO(arthurb): I'd write a fix for this, but I have no idea what it means
	if b.config.ImportOpts != "" {
		b.config.ImportFlags = append(b.config.ImportFlags, "--options", b.config.ImportOpts)
	}

	validMode := false
	validModes := []string{
		vboxcommon.GuestAdditionsModeDisable,
		vboxcommon.GuestAdditionsModeAttach,
		vboxcommon.GuestAdditionsModeUpload,
	}

	for _, mode := range validModes {
		if b.config.GuestAdditionsMode == mode {
			validMode = true
			break
		}
	}

	if !validMode {
		errs = packer.MultiErrorAppend(errs,
			fmt.Errorf("guest_additions_mode is invalid. Must be one of: %v", validModes))
	}

	if b.config.GuestAdditionsSHA256 != "" {
		b.config.GuestAdditionsSHA256 = strings.ToLower(b.config.GuestAdditionsSHA256)
	}

	// Warnings
	if b.config.ShutdownCommand == "" {
		warnings = append(warnings,
			"A shutdown_command was not specified. Without a shutdown command, Packer\n"+
				"will forcibly halt the virtual machine, which may result in data loss.")
	}

	if errs != nil && len(errs.Errors) > 0 {
		return warnings, errs
	}

	return warnings, nil
}

// Run executes a Packer build and returns a packer.Artifact representing
// a VirtualBox appliance.
func (b *Builder) Run(ui packer.Ui, hook packer.Hook, cache packer.Cache) (packer.Artifact, error) {
	// Create the driver that we'll use to communicate with VirtualBox
	driver, err := vboxcommon.NewDriver()
	if err != nil {
		return nil, fmt.Errorf("Failed creating VirtualBox driver: %s", err)
	}

	// Build the steps.
	steps := []multistep.Step{
		&vboxcommon.StepOutputDir{
			Force: b.config.PackerForce,
			Path:  b.config.OutputDir,
		},
		new(vboxcommon.StepSuppressMessages),
		&common.StepCreateFloppy{
			Files:       b.config.FloppyConfig.FloppyFiles,
			Directories: b.config.FloppyConfig.FloppyDirectories,
		},
		&common.StepHTTPServer{
			HTTPDir:     b.config.HTTPDir,
			HTTPPortMin: b.config.HTTPPortMin,
			HTTPPortMax: b.config.HTTPPortMax,
		},
		&vboxcommon.StepDownloadGuestAdditions{
			GuestAdditionsMode:   b.config.GuestAdditionsMode,
			GuestAdditionsURL:    b.config.GuestAdditionsURL,
			GuestAdditionsSHA256: b.config.GuestAdditionsSHA256,
			Ctx:                  b.config.ctx,
		},
		&common.StepDownload{
			Checksum:     b.config.Checksum,
			ChecksumType: b.config.ChecksumType,
			Description:  "OVF/OVA",
			Extension:    "ova",
			ResultKey:    "vm_path",
			TargetPath:   b.config.TargetPath,
			Url:          []string{b.config.SourcePath},
		},
		&StepImport{
			Name:        b.config.VMName,
			SourcePath:  b.config.SourcePath,
			ImportFlags: b.config.ImportFlags,
		},
		&vboxcommon.StepAttachGuestAdditions{
			GuestAdditionsMode: b.config.GuestAdditionsMode,
		},
		&vboxcommon.StepConfigureVRDP{
			VRDPBindAddress: b.config.VRDPBindAddress,
			VRDPPortMin:     b.config.VRDPPortMin,
			VRDPPortMax:     b.config.VRDPPortMax,
		},
		new(vboxcommon.StepAttachFloppy),
		&vboxcommon.StepForwardSSH{
			CommConfig:     &b.config.SSHConfig.Comm,
			HostPortMin:    b.config.SSHHostPortMin,
			HostPortMax:    b.config.SSHHostPortMax,
			SkipNatMapping: b.config.SSHSkipNatMapping,
		},
		&vboxcommon.StepVBoxManage{
			Commands: b.config.VBoxManage,
			Ctx:      b.config.ctx,
		},
		&vboxcommon.StepRun{
			BootWait: b.config.BootWait,
			Headless: b.config.Headless,
		},
		&vboxcommon.StepTypeBootCommand{
			BootCommand: b.config.BootCommand,
			VMName:      b.config.VMName,
			Ctx:         b.config.ctx,
		},
		&communicator.StepConnect{
			Config:    &b.config.SSHConfig.Comm,
			Host:      vboxcommon.CommHost(b.config.SSHConfig.Comm.SSHHost),
			SSHConfig: vboxcommon.SSHConfigFunc(b.config.SSHConfig),
			SSHPort:   vboxcommon.SSHPort,
		},
		&vboxcommon.StepUploadVersion{
			Path: b.config.VBoxVersionFile,
		},
		&vboxcommon.StepUploadGuestAdditions{
			GuestAdditionsMode: b.config.GuestAdditionsMode,
			GuestAdditionsPath: b.config.GuestAdditionsPath,
			Ctx:                b.config.ctx,
		},
		new(common.StepProvision),
		&vboxcommon.StepShutdown{
			Command: b.config.ShutdownCommand,
			Timeout: b.config.ShutdownTimeout,
			Delay: b.config.PostShutdownDelay,
		},
		new(vboxcommon.StepRemoveDevices),
		&vboxcommon.StepVBoxManage{
			Commands: b.config.VBoxManagePost,
			Ctx:      b.config.ctx,
		},
		&vboxcommon.StepExport{
			Format:         b.config.Format,
			OutputDir:      b.config.OutputDir,
			ExportOpts:     b.config.ExportOpts.ExportOpts,
			SkipNatMapping: b.config.SSHSkipNatMapping,
		},
	}

	// Set up the state.
	state := new(multistep.BasicStateBag)
	state.Put("config", b.config)
	state.Put("debug", b.config.PackerDebug)
	state.Put("driver", driver)
	state.Put("cache", cache)
	state.Put("hook", hook)
	state.Put("ui", ui)

	// Run the steps.
	b.runner = common.NewRunnerWithPauseFn(steps, b.config.PackerConfig, ui, state)
	b.runner.Run(state)

	// Report any errors.
	if rawErr, ok := state.GetOk("error"); ok {
		return nil, rawErr.(error)
	}

	// If we were interrupted or cancelled, then just exit.
	if _, ok := state.GetOk(multistep.StateCancelled); ok {
		return nil, errors.New("Build was cancelled.")
	}

	if _, ok := state.GetOk(multistep.StateHalted); ok {
		return nil, errors.New("Build was halted.")
	}

	return vboxcommon.NewArtifact(b.config.OutputDir)
}

// Cancel.
func (b *Builder) Cancel() {
	if b.runner != nil {
		log.Println("Cancelling the step runner...")
		b.runner.Cancel()
	}
}
