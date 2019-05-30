package hcsv2

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Microsoft/opengcs/internal/network"
	oci "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

func getSandboxRootDir(id string) string {
	return filepath.Join("/tmp/gcs/cri", id)
}

func getSandboxHostnamePath(id string) string {
	return filepath.Join(getSandboxRootDir(id), "hostname")
}

func getSandboxHostsPath(id string) string {
	return filepath.Join(getSandboxRootDir(id), "hosts")
}

func getSandboxResolvPath(id string) string {
	return filepath.Join(getSandboxRootDir(id), "resolv.conf")
}

func setupSandboxContainerSpec(ctx context.Context, id string, spec *oci.Spec) (err error) {
	// TODO: JTERRY75 use ctx for log
	operation := "setupSandboxContainerSpec"
	start := time.Now()
	defer func() {
		end := time.Now()
		fields := logrus.Fields{
			"cid":       id,
			"startTime": start,
			"endTime":   end,
			"duration":  end.Sub(start),
		}
		if err != nil {
			fields[logrus.ErrorKey] = err
			logrus.WithFields(fields).Error(operation)
		} else {
			logrus.WithFields(fields).Info(operation)
		}
	}()

	// Generate the sandbox root dir
	rootDir := getSandboxRootDir(id)
	if err := os.MkdirAll(rootDir, 0755); err != nil {
		return errors.Wrapf(err, "failed to create sandbox root directory %q", rootDir)
	}

	// Write the hostname
	hostname := spec.Hostname
	if hostname == "" {
		var err error
		hostname, err = os.Hostname()
		if err != nil {
			return errors.Wrap(err, "failed to get hostname")
		}
	}

	sandboxHostnamePath := getSandboxHostnamePath(id)
	if err := ioutil.WriteFile(sandboxHostnamePath, []byte(hostname+"\n"), 0644); err != nil {
		return errors.Wrapf(err, "failed to write hostname to %q", sandboxHostnamePath)
	}

	// Write the hosts
	sandboxHostsPath := getSandboxHostsPath(id)
	if err := copyFile("/etc/hosts", sandboxHostsPath, 0644); err != nil {
		return errors.Wrapf(err, "failed to write sandbox hosts to %q", sandboxHostsPath)
	}

	// Write resolv.conf
	ns, err := getNetworkNamespace(getNetworkNamespaceID(spec))
	if err != nil {
		return err
	}
	var searches, servers []string
	for _, n := range ns.Adapters() {
		searches = network.MergeValues(searches, strings.Split(n.DNSSuffix, ","))
		servers = network.MergeValues(servers, strings.Split(n.DNSServerList, ","))
	}
	resolvContent, err := network.GenerateResolvConfContent(ctx, searches, servers, nil)
	if err != nil {
		return errors.Wrap(err, "failed to generate sandbox resolv.conf content")
	}
	sandboxResolvPath := getSandboxResolvPath(id)
	if err := ioutil.WriteFile(sandboxResolvPath, []byte(resolvContent), 0644); err != nil {
		return errors.Wrap(err, "failed to write sandbox resolv.conf")
	}

	// TODO: JTERRY75 /dev/shm is not properly setup for LCOW I believe. CRI
	// also has a concept of a sandbox/shm file when the IPC NamespaceMode !=
	// NODE.

	// Clear the windows section as we dont want to forward to runc
	spec.Windows = nil

	return nil
}
