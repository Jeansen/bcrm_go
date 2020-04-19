package main

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/pborman/getopt/v2"
)

type arguments struct {
	Src              *string
	Dest             *string
	Uefi             *bool
	Help             *bool
	destImg          *[]string
	srctImg          *[]string
	DestImg          bcrmImg
	SrctImg          bcrmImg
	NewVgName        *string
	EncryptPw        *string
	Hostname         *string
	MakeUefi         *bool
	UseAllPvs        *bool
	Quiet            *bool
	Split            *bool
	Check            *bool
	Compress         *bool
	ResizeThreshold  *string
	SwapSize         *string
	BootSize         *string
	LvmExpand        *string
	VgFreeSize       *string
	RemovePkgs       *[]string
	Schroot          *bool
	DisableMount     *string
	NoCleanup        *bool
	AllToLvm         *bool
	IncludePartition *[]string
	ToLvm            *string
}

type bcrmImg struct {
	Path      string
	Type      string
	CanonSize string
	SizeMB    int
}

var hidden = regexp.MustCompile(`^\..*`)

var usage = `
    Usage: $(basename $0) -s <source> -d <destination> [options]

    OPTIONS
    -------
    -s, --source                 The source device or folder to clone or restore from 
    -d, --destination            The destination device or folder to clone or backup to 
        --source-image           Use the given image as source in the form of <path>:<type> 
                                 For example: '/path/to/file.vdi:vdi'. See below for supported types. 
        --destination-image      Use the given image as destination in the form of <path>:<type>[:<virtual-size>] 
                                 For instance: '/path/to/file.img:raw:20G' 
                                 If you omit the size, the image file must exists. 
                                 If you provide a size, the image file will be created or overwritten. 
    -c, --check                  Create/Validate checksums 
    -z, --compress               Use compression (compression ratio is about 1:3, but very slow!) 
        --split                  Split backup into chunks of 1G files 
    -H, --hostname               Set hostname 
        --remove-pkgs            Remove the given list of whitespace-separated packages as a final step. 
                                 The whole list must be enclosed in ""
    -n, --new-vg-name            LVM only: Define new volume group name 
        --vg-free-size           LVM only: How much space should be added to remaining free space in source VG. 
    -e, --encrypt-with-password  LVM only: Create encrypted disk with supplied passphrase 
    -p, --use-all-pvs            LVM only: Use all disks found on destination as PVs for VG 
        --lvm-expand             LVM only: Have the given LV use the remaining free space. 
                                 An optional percentage can be supplied, e.g. 'root:80' 
                                 Which would add 80% of the remaining free space in a VG to this LV 
    -u, --make-uefi              Convert to UEFI 
    -w, --swap-size              Swap partition size. May be zero to remove any swap partition. 
    -m, --resize-threshold       Do not resize partitions smaller than <size> (default 2048M) 
        --schroot                Run in a secure chroot environment with a fixed and tested tool chain 
        --no-cleanup             Do not remove temporary (backup) files and mounts. 
                                 Useful when tracking down errors with --schroot. 
        --disable-mount          Disable the given mount point in <destination>/etc/fstab. 
                                 For instance --disable-mount /some/path. Can be used multiple times. 
        --to-lvm                 Convert given source partition to LV. E.g. '/dev/sda1:boot' would be 
                                 converted to LV with the name 'boot' Can be used multiple times. 
                                 Only works for partitions that have a valid mountpoint in fstab 
        --all-to-lvm             Convert all source partitions to LV. (except EFI) 
        --include-partition      Also include the content of the given partition to the specified path. 
                                 E.g: 'part=/dev/sdX,dir=/some/path/,user=1000,group=10001,exclude=fodler1,folder2' 
                                 would copy all content from /dev/sdX to /some/path. 
                                 If /some/path does not exist, it will be created with the given user 
                                 and group ID, or root otherwise. With exclude you can filter folders and files. 
                                 This option can be specified multiple times. 
    -q, --quiet                  Quiet, do not show any output 
    -h, --help                   Show this help text 

   
    ADVANCED OPTIONS
    ----------------
    -b, --boot-size               Boot partition size. For instance: 200M or 4G. 
                                  Be careful, the  script only checks for the bootable flag, 
                                  Only use with a dedicated /boot partition 

    ADDITIONAL NOTES
    ----------------
    Size values must be postfixed with a size indcator, e.g: 200M or 4G. The following indicators are valid:

    K [kilobytes]
    M [megabytes]
    G [gigabytes]
    T [terabytes]

    When using virtual images you always have to provide the image type. Currently the following image types are supported:

    raw    Plain binary 
    vdi    Virtual Box 
    qcow2  QEMU/KVM 
    vmdk   VMware 
    vhdx   Hyper-V   
`
var args = arguments{}

func init() {
	getopt.HelpColumn = 60
	getopt.DisplayWidth = 70

	help := func() {
		fmt.Println(usage)
	}

	getopt.SetUsage(help)

	args.Help = getopt.BoolLong("help", 'h')
	args.Src = getopt.StringLong("source", 's', "")
	args.srctImg = getopt.ListLong("source-image", 'S')
	args.destImg = getopt.ListLong("destination-image", 'D')
	args.Dest = getopt.StringLong("destination", 'd', "")
	args.NewVgName = getopt.StringLong("new-vg-name", 'n', "")
	args.EncryptPw = getopt.StringLong("encrypt-with-password", 'e', "")
	args.Hostname = getopt.StringLong("hostname", 'H', "")
	args.MakeUefi = getopt.BoolLong("make-uefi", 'u')
	args.UseAllPvs = getopt.BoolLong("use-all-pvs", 'p')
	args.Quiet = getopt.BoolLong("quiet", 'q')
	args.Split = getopt.BoolLong("split", 'l')
	args.Check = getopt.BoolLong("check", 'c')
	args.Compress = getopt.BoolLong("compress", 'z')
	args.ResizeThreshold = getopt.StringLong("resize-threshold", 'm', "")
	args.SwapSize = getopt.StringLong("swap-size", 'w', "")
	args.BootSize = getopt.StringLong("boot-size", 'b', "")
	args.LvmExpand = getopt.StringLong("lvm-expand", 'E', "")
	args.VgFreeSize = getopt.StringLong("vg-free-size", 'F', "")
	args.RemovePkgs = getopt.ListLong("remove-pkgs", 'R')
	args.Schroot = getopt.BoolLong("schroot", 'T')
	args.DisableMount = getopt.StringLong("disable-mount", 'M', "")
	args.NoCleanup = getopt.BoolLong("no-cleanup", 'C')
	args.AllToLvm = getopt.BoolLong("all-to-lvm", 'A')
	args.IncludePartition = getopt.ListLong("include-partition", 'I')
	args.ToLvm = getopt.StringLong("to-lvm", 'L', "")

	//if err := fs.Getopt(os.Args, nil); err != nil {
	//	fmt.Fprintf(os.Stderr, "%v\n", err)
	//	os.Exit(1)
	//}

	//
	//if *help {
	//	fs.PrintUsage(os.Stderr)
	//	os.Exit(0)
	//}
	//
	//if err := fs.Getopt(os.Args, nil); err != nil {
	//	fmt.Fprintln(os.Stderr, err)
	//}
}

func main() {
	getopt.Parse()
	fmt.Println(args.validate(getopt.CommandLine))
}

func isAccessable(fi os.FileInfo) bool {
	sy := fi.Sys()
	g := sy.(*syscall.Stat_t).Mode
	if g&unix.S_IXUSR > 0 && isCurrentUser(fi) ||
		g&unix.S_IXGRP > 0 && isCurrentGroup(fi) ||
		g&unix.S_IXOTH > 0 {
		return true
	}
	return false
}

func isReadable(fi os.FileInfo) bool {
	sy := fi.Sys()
	g := sy.(*syscall.Stat_t).Mode
	if g&unix.S_IRUSR > 0 && isCurrentUser(fi) ||
		g&unix.S_IRGRP > 0 && isCurrentGroup(fi) ||
		g&unix.S_IROTH > 0 {
		return true
	}
	return false
}

func isEmptyDir(file string) bool {
	var files []string
	statFile, _ := os.Stat(file)

	err := filepath.Walk(file, func(path string, info os.FileInfo, err error) error {
		if os.SameFile(statFile, info) {
			return nil
		}
		if hidden.MatchString(filepath.Base(path)) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		panic(err)
	}
	if len(files) > 0 {
		return false
	}
	return true
}

func validatePath(file string) error {
	if len(file) == 0 {
		return errors.New("Parameter value is empty.")
	}
	if srcm, ok := os.Stat(file); ok == nil {
		if mode := srcm.Mode(); mode&os.ModeDevice == 0 && mode&os.ModeDir == 0 {
			return errors.New("Invalid Folder or device.")
		}
	} else {
		return errors.New("Folder or device does not exist.")
	}
	return nil
}

func (args *arguments) validate(params *getopt.Set) error {
	_checkArguments := func() error {
		if *args.Help {
			params.PrintUsage(os.Stderr)
			os.Exit(0)
		}
		if !params.IsSet("source") {
			return errors.New("Missing required option -s <source>")
		}
		if err := validatePath(*args.Src); err != nil {
			return errors.New("-s: " + error.Error(err))
		}
		if !params.IsSet("destination") {
			return errors.New("Missing required option -d <destinaton>")
		}
		if err := validatePath(*args.Dest); err != nil {
			return errors.New("-s: " + error.Error(err))
		}
		return nil
	}

	_check := func() error {
		srcm, _ := os.Stat(*args.Src)
		destm, _ := os.Stat(*args.Dest)

		smode := srcm.Mode()
		dmode := destm.Mode()

		if os.SameFile(srcm, destm) {
			return errors.New("Source and destination cannot be the same!")
		}
		if smode&os.ModeDir != 0 && dmode&os.ModeDevice != 0 && isEmptyDir(*args.Src) {
			return errors.New("No backup available. Source is empty!")
		}
		if smode&os.ModeDevice != 0 && dmode&os.ModeDir != 0 && !isEmptyDir(*args.Dest) {
			return errors.New("Destination not empty!")
		}
		if smode&os.ModeDir != 0 && dmode&os.ModeDevice == 0 {
			return errors.New(*args.Dest + " is not a valid block device")
		}
		if smode&os.ModeDevice == 0 && dmode&os.ModeDir != 0 {
			return errors.New(*args.Src + " is not a valid block device")
		}
		if smode&os.ModeDevice == 0 && smode&os.ModeDir == 0 && dmode&os.ModeDir != 0 {
			return errors.New("Invalid device or directory: " + *args.Src)
		}
		if smode&os.ModeDir != 0 && dmode&os.ModeDevice == 0 && dmode&os.ModeDir == 0 {
			return errors.New("Invalid device or directory: " + *args.Dest)
		}

		if smode&os.ModeDir != 0 && !isReadable(srcm) {
			return errors.New(*args.Src + " is not readable")
		}
		if dmode&os.ModeDir != 0 && !isAccessable(destm) {
			return errors.New(*args.Dest + " is not writable")
		}
		return nil
	}

	if err := _checkArguments(); err != nil {
		return err
	}
	if err := _check(); err != nil {
		return err
	}
	return nil
}

func isCurrentGroup(fi os.FileInfo) bool {
	u, _ := user.Current()
	sy := fi.Sys()
	g := sy.(*syscall.Stat_t).Gid

	gs, _ := u.GroupIds()
	for _, v := range gs {
		if t, _ := strconv.ParseUint(v, 10, 32); uint32(t) == g {
			return true
		}
	}
	return false
}

func isCurrentUser(fi os.FileInfo) bool {
	u, _ := user.Current()
	sy := fi.Sys()
	g := sy.(*syscall.Stat_t).Uid

	if t, _ := strconv.ParseUint(u.Uid, 10, 32); uint32(t) == g {
		return true
	}
	return false
}
