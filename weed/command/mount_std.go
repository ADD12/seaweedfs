// +build linux darwin

package command

import (
	"fmt"
	"runtime"

	"bazil.org/fuse"
	"bazil.org/fuse/fs"
	"github.com/chrislusf/seaweedfs/weed/filesys"
	"github.com/chrislusf/seaweedfs/weed/glog"
	"github.com/chrislusf/seaweedfs/weed/util"
	"strings"
	"strconv"
)

func runMount(cmd *Command, args []string) bool {
	fmt.Printf("This is SeaweedFS version %s %s %s\n", util.VERSION, runtime.GOOS, runtime.GOARCH)
	if *mountOptions.dir == "" {
		fmt.Printf("Please specify the mount directory via \"-dir\"")
		return false
	}
	if *mountOptions.chunkSizeLimitMB <= 0 {
		fmt.Printf("Please specify a reasonable buffer size.")
		return false
	}

	fuse.Unmount(*mountOptions.dir)

	c, err := fuse.Mount(
		*mountOptions.dir,
		fuse.VolumeName("SeaweedFS"),
		fuse.FSName("SeaweedFS"),
		fuse.NoAppleDouble(),
		fuse.NoAppleXattr(),
		fuse.ExclCreate(),
		fuse.DaemonTimeout("3600"),
		fuse.AllowOther(),
		fuse.AllowSUID(),
		fuse.DefaultPermissions(),
		// fuse.MaxReadahead(1024*128), // TODO: not tested yet, possibly improving read performance
		fuse.AsyncRead(),
		fuse.WritebackCache(),
	)
	if err != nil {
		glog.Fatal(err)
		return false
	}

	util.OnInterrupt(func() {
		fuse.Unmount(*mountOptions.dir)
		c.Close()
	})

	filerGrpcAddress, err := parseFilerGrpcAddress(*mountOptions.filer, *mountOptions.filerGrpcPort)
	if err != nil {
		glog.Fatal(err)
		return false
	}

	err = fs.Serve(c, filesys.NewSeaweedFileSystem(
		filerGrpcAddress, *mountOptions.filerMountRootPath, *mountOptions.collection, *mountOptions.replication, int32(*mountOptions.ttlSec),
		*mountOptions.chunkSizeLimitMB, *mountOptions.dataCenter))
	if err != nil {
		fuse.Unmount(*mountOptions.dir)
	}

	// check if the mount process has an error to report
	<-c.Ready
	if err := c.MountError; err != nil {
		glog.Fatal(err)
	}

	return true
}

func parseFilerGrpcAddress(filer string, optionalGrpcPort int) (filerGrpcAddress string, err error) {
	hostnameAndPort := strings.Split(filer, ":")
	if len(hostnameAndPort) != 2 {
		return "", fmt.Errorf("The filer should have hostname:port format: %v", hostnameAndPort)
	}

	filerPort, parseErr := strconv.ParseUint(hostnameAndPort[1], 10, 64)
	if parseErr != nil {
		return "", fmt.Errorf("The filer filer port parse error: %v", parseErr)
	}

	filerGrpcPort := int(filerPort) + 10000
	if optionalGrpcPort != 0 {
		filerGrpcPort = optionalGrpcPort
	}

	return fmt.Sprintf("%s:%d", hostnameAndPort[0], filerGrpcPort), nil
}
