/*
 * Minio Cloud Storage, (C) 2014, 2015, 2016, 2017, 2018 Minio, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package cmd

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/minio/minio/cmd/logger"
	xnet "github.com/minio/minio/pkg/net"
)

var errUnsupportedSignal = fmt.Errorf("unsupported signal: only restart and stop signals are supported")

// AdminRPCClient - admin RPC client talks to admin RPC server.
type AdminRPCClient struct {
	*RPCClient
}

// SignalService - calls SignalService RPC.
func (rpcClient *AdminRPCClient) SignalService(signal serviceSignal) (err error) {
	args := SignalServiceArgs{Sig: signal}
	reply := VoidReply{}

	return rpcClient.Call(adminServiceName+".SignalService", &args, &reply)
}

// ReInitFormat - re-initialize disk format, remotely.
func (rpcClient *AdminRPCClient) ReInitFormat(dryRun bool) error {
	args := ReInitFormatArgs{DryRun: dryRun}
	reply := VoidReply{}

	return rpcClient.Call(adminServiceName+".ReInitFormat", &args, &reply)
}

// ServerInfo - returns the server info of the server to which the RPC call is made.
func (rpcClient *AdminRPCClient) ServerInfo() (sid ServerInfoData, err error) {
	err = rpcClient.Call(adminServiceName+".ServerInfo", &AuthArgs{}, &sid)
	return sid, err
}

// GetConfig - returns config.json of the remote server.
func (rpcClient *AdminRPCClient) GetConfig() ([]byte, error) {
	args := AuthArgs{}
	var reply []byte

	err := rpcClient.Call(adminServiceName+".GetConfig", &args, &reply)
	return reply, err
}

// WriteTmpConfig - writes config file content to a temporary file on a remote node.
func (rpcClient *AdminRPCClient) WriteTmpConfig(tmpFileName string, configBytes []byte) error {
	args := WriteConfigArgs{
		TmpFileName: tmpFileName,
		Buf:         configBytes,
	}
	reply := VoidReply{}

	err := rpcClient.Call(adminServiceName+".WriteTmpConfig", &args, &reply)
	logger.LogIf(context.Background(), err)
	return err
}

// CommitConfig - Move the new config in tmpFileName onto config.json on a remote node.
func (rpcClient *AdminRPCClient) CommitConfig(tmpFileName string) error {
	args := CommitConfigArgs{FileName: tmpFileName}
	reply := VoidReply{}

	err := rpcClient.Call(adminServiceName+".CommitConfig", &args, &reply)
	logger.LogIf(context.Background(), err)
	return err
}

// NewAdminRPCClient - returns new admin RPC client.
func NewAdminRPCClient(host *xnet.Host) (*AdminRPCClient, error) {
	scheme := "http"
	if globalIsSSL {
		scheme = "https"
	}

	serviceURL := &xnet.URL{
		Scheme: scheme,
		Host:   host.String(),
		Path:   adminServicePath,
	}

	var tlsConfig *tls.Config
	if globalIsSSL {
		tlsConfig = &tls.Config{
			ServerName: host.Name,
			RootCAs:    globalRootCAs,
		}
	}

	rpcClient, err := NewRPCClient(
		RPCClientArgs{
			NewAuthTokenFunc: newAuthToken,
			RPCVersion:       globalRPCAPIVersion,
			ServiceName:      adminServiceName,
			ServiceURL:       serviceURL,
			TLSConfig:        tlsConfig,
		},
	)
	if err != nil {
		return nil, err
	}

	return &AdminRPCClient{rpcClient}, nil
}

// adminCmdRunner - abstracts local and remote execution of admin
// commands like service stop and service restart.
type adminCmdRunner interface {
	SignalService(s serviceSignal) error
	ReInitFormat(dryRun bool) error
	ServerInfo() (ServerInfoData, error)
	GetConfig() ([]byte, error)
	WriteTmpConfig(tmpFileName string, configBytes []byte) error
	CommitConfig(tmpFileName string) error
}

// adminPeer - represents an entity that implements admin API RPCs.
type adminPeer struct {
	addr      string
	cmdRunner adminCmdRunner
	isLocal   bool
}

// type alias for a collection of adminPeer.
type adminPeers []adminPeer

// makeAdminPeers - helper function to construct a collection of adminPeer.
func makeAdminPeers(endpoints EndpointList) (adminPeerList adminPeers) {
	localAddr := GetLocalPeer(endpoints)
	if strings.HasPrefix(localAddr, "127.0.0.1:") {
		// Use first IPv4 instead of loopback address.
		localAddr = net.JoinHostPort(sortIPs(localIP4.ToSlice())[0], globalMinioPort)
	}
	adminPeerList = append(adminPeerList, adminPeer{
		addr:      localAddr,
		cmdRunner: localAdminClient{},
		isLocal:   true,
	})

	for _, hostStr := range GetRemotePeers(endpoints) {
		host, err := xnet.ParseHost(hostStr)
		logger.FatalIf(err, "Unable to parse Admin RPC Host", context.Background())
		rpcClient, err := NewAdminRPCClient(host)
		logger.FatalIf(err, "Unable to initialize Admin RPC Client", context.Background())
		adminPeerList = append(adminPeerList, adminPeer{
			addr:      hostStr,
			cmdRunner: rpcClient,
		})
	}

	return adminPeerList
}

// peersReInitFormat - reinitialize remote object layers to new format.
func peersReInitFormat(peers adminPeers, dryRun bool) error {
	errs := make([]error, len(peers))

	// Send ReInitFormat RPC call to all nodes.
	// for local adminPeer this is a no-op.
	wg := sync.WaitGroup{}
	for i, peer := range peers {
		wg.Add(1)
		go func(idx int, peer adminPeer) {
			defer wg.Done()
			if !peer.isLocal {
				errs[idx] = peer.cmdRunner.ReInitFormat(dryRun)
			}
		}(i, peer)
	}
	wg.Wait()
	return nil
}

// Initialize global adminPeer collection.
func initGlobalAdminPeers(endpoints EndpointList) {
	globalAdminPeers = makeAdminPeers(endpoints)
}

// invokeServiceCmd - Invoke Restart/Stop command.
func invokeServiceCmd(cp adminPeer, cmd serviceSignal) (err error) {
	switch cmd {
	case serviceRestart, serviceStop:
		err = cp.cmdRunner.SignalService(cmd)
	}
	return err
}

// sendServiceCmd - Invoke Restart command on remote peers
// adminPeer followed by on the local peer.
func sendServiceCmd(cps adminPeers, cmd serviceSignal) {
	// Send service command like stop or restart to all remote nodes and finally run on local node.
	errs := make([]error, len(cps))
	var wg sync.WaitGroup
	remotePeers := cps[1:]
	for i := range remotePeers {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			// we use idx+1 because remotePeers slice is 1 position shifted w.r.t cps
			errs[idx+1] = invokeServiceCmd(remotePeers[idx], cmd)
		}(i)
	}
	wg.Wait()
	errs[0] = invokeServiceCmd(cps[0], cmd)
}

// uptimeSlice - used to sort uptimes in chronological order.
type uptimeSlice []struct {
	err    error
	uptime time.Duration
}

func (ts uptimeSlice) Len() int {
	return len(ts)
}

func (ts uptimeSlice) Less(i, j int) bool {
	return ts[i].uptime < ts[j].uptime
}

func (ts uptimeSlice) Swap(i, j int) {
	ts[i], ts[j] = ts[j], ts[i]
}

// getPeerUptimes - returns the uptime since the last time read quorum
// was established on success. Otherwise returns errXLReadQuorum.
func getPeerUptimes(peers adminPeers) (time.Duration, error) {
	// In a single node Erasure or FS backend setup the uptime of
	// the setup is the uptime of the single minio server
	// instance.
	if !globalIsDistXL {
		return UTCNow().Sub(globalBootTime), nil
	}

	uptimes := make(uptimeSlice, len(peers))

	// Get up time of all servers.
	wg := sync.WaitGroup{}
	for i, peer := range peers {
		wg.Add(1)
		go func(idx int, peer adminPeer) {
			defer wg.Done()
			serverInfoData, rpcErr := peer.cmdRunner.ServerInfo()
			uptimes[idx].uptime, uptimes[idx].err = serverInfoData.Properties.Uptime, rpcErr
		}(i, peer)
	}
	wg.Wait()

	// Sort uptimes in chronological order.
	sort.Sort(uptimes)

	// Pick the readQuorum'th uptime in chronological order. i.e,
	// the time at which read quorum was (re-)established.
	readQuorum := len(uptimes) / 2
	validCount := 0
	latestUptime := time.Duration(0)
	for _, uptime := range uptimes {
		if uptime.err != nil {
			logger.LogIf(context.Background(), uptime.err)
			continue
		}

		validCount++
		if validCount >= readQuorum {
			latestUptime = uptime.uptime
			break
		}
	}

	// Less than readQuorum "Admin.Uptime" RPC call returned
	// successfully, so read-quorum unavailable.
	if validCount < readQuorum {
		return time.Duration(0), InsufficientReadQuorum{}
	}

	return latestUptime, nil
}

// getPeerConfig - Fetches config.json from all nodes in the setup and
// returns the one that occurs in a majority of them.
func getPeerConfig(peers adminPeers) ([]byte, error) {
	if !globalIsDistXL {
		return peers[0].cmdRunner.GetConfig()
	}

	errs := make([]error, len(peers))
	configs := make([][]byte, len(peers))

	// Get config from all servers.
	wg := sync.WaitGroup{}
	for i, peer := range peers {
		wg.Add(1)
		go func(idx int, peer adminPeer) {
			defer wg.Done()
			configs[idx], errs[idx] = peer.cmdRunner.GetConfig()
		}(i, peer)
	}
	wg.Wait()

	// Find the maximally occurring config among peers in a
	// distributed setup.

	serverConfigs := make([]serverConfig, len(peers))
	for i, configBytes := range configs {
		if errs[i] != nil {
			continue
		}

		// Unmarshal the received config files.
		err := json.Unmarshal(configBytes, &serverConfigs[i])
		if err != nil {
			reqInfo := (&logger.ReqInfo{}).AppendTags("peerAddress", peers[i].addr)
			ctx := logger.SetReqInfo(context.Background(), reqInfo)
			logger.LogIf(ctx, err)
			return nil, err
		}
	}

	configJSON, err := getValidServerConfig(serverConfigs, errs)
	if err != nil {
		logger.LogIf(context.Background(), err)
		return nil, err
	}

	// Return the config.json that was present quorum or more
	// number of disks.
	return json.Marshal(configJSON)
}

// getValidServerConfig - finds the server config that is present in
// quorum or more number of servers.
func getValidServerConfig(serverConfigs []serverConfig, errs []error) (scv serverConfig, e error) {
	// majority-based quorum
	quorum := len(serverConfigs)/2 + 1

	// Count the number of disks a config.json was found in.
	configCounter := make([]int, len(serverConfigs))

	// We group equal serverConfigs by the lowest index of the
	// same value;  e.g, let us take the following serverConfigs
	// in a 4-node setup,
	// serverConfigs == [c1, c2, c1, c1]
	// configCounter == [3, 1, 0, 0]
	// c1, c2 are the only distinct values that appear.  c1 is
	// identified by 0, the lowest index it appears in and c2 is
	// identified by 1. So, we need to find the number of times
	// each of these distinct values occur.

	// Invariants:

	// 1. At the beginning of the i-th iteration, the number of
	// unique configurations seen so far is equal to the number of
	// non-zero counter values in config[:i].

	// 2. At the beginning of the i-th iteration, the sum of
	// elements of configCounter[:i] is equal to the number of
	// non-error configurations seen so far.

	// For each of the serverConfig ...
	for i := range serverConfigs {
		// Skip nodes where getConfig failed.
		if errs[i] != nil {
			continue
		}
		// Check if it is equal to any of the configurations
		// seen so far. If j == i is reached then we have an
		// unseen configuration.
		for j := 0; j <= i; j++ {
			if j < i && configCounter[j] == 0 {
				// serverConfigs[j] is known to be
				// equal to a value that was already
				// seen. See example above for
				// clarity.
				continue
			} else if j < i && serverConfigs[i].ConfigDiff(&serverConfigs[j]) == "" {
				// serverConfigs[i] is equal to
				// serverConfigs[j], update
				// serverConfigs[j]'s counter since it
				// is the lower index.
				configCounter[j]++
				break
			} else if j == i {
				// serverConfigs[i] is equal to no
				// other value seen before. It is
				// unique so far.
				configCounter[i] = 1
				break
			} // else invariants specified above are violated.
		}
	}

	// We find the maximally occurring server config and check if
	// there is quorum.
	var configJSON serverConfig
	maxOccurrence := 0
	for i, count := range configCounter {
		if maxOccurrence < count {
			maxOccurrence = count
			configJSON = serverConfigs[i]
		}
	}

	// If quorum nodes don't agree.
	if maxOccurrence < quorum {
		return scv, errXLWriteQuorum
	}

	return configJSON, nil
}

// Write config contents into a temporary file on all nodes.
func writeTmpConfigPeers(peers adminPeers, tmpFileName string, configBytes []byte) []error {
	// For a single-node minio server setup.
	if !globalIsDistXL {
		err := peers[0].cmdRunner.WriteTmpConfig(tmpFileName, configBytes)
		return []error{err}
	}

	errs := make([]error, len(peers))

	// Write config into temporary file on all nodes.
	wg := sync.WaitGroup{}
	for i, peer := range peers {
		wg.Add(1)
		go func(idx int, peer adminPeer) {
			defer wg.Done()
			errs[idx] = peer.cmdRunner.WriteTmpConfig(tmpFileName, configBytes)
		}(i, peer)
	}
	wg.Wait()

	// Return bytes written and errors (if any) during writing
	// temporary config file.
	return errs
}

// Move config contents from the given temporary file onto config.json
// on all nodes.
func commitConfigPeers(peers adminPeers, tmpFileName string) []error {
	// For a single-node minio server setup.
	if !globalIsDistXL {
		return []error{peers[0].cmdRunner.CommitConfig(tmpFileName)}
	}

	errs := make([]error, len(peers))

	// Rename temporary config file into configDir/config.json on
	// all nodes.
	wg := sync.WaitGroup{}
	for i, peer := range peers {
		wg.Add(1)
		go func(idx int, peer adminPeer) {
			defer wg.Done()
			errs[idx] = peer.cmdRunner.CommitConfig(tmpFileName)
		}(i, peer)
	}
	wg.Wait()

	// Return errors (if any) received during rename.
	return errs
}
