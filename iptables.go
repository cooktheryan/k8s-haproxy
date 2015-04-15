/*
Copyright 2014 Google Inc. All rights reserved.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/GoogleCloudPlatform/kubernetes/pkg/api"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/types"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util/errors"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util/exec"
	"github.com/GoogleCloudPlatform/kubernetes/pkg/util/iptables"
	"github.com/golang/glog"
)

var (
	listenIP = zeroIPv4
	hostIP   = zeroIPv4

	ipt = iptables.New(exec.New(), iptables.ProtocolIpv4)
)

// ServicePortName carries a namespace + name + portname.  This is the unique
// identfier for a load-balanced service.
type ServicePortName struct {
	types.NamespacedName
	Port string
}

type ServiceInfo struct {
	PortalIP   net.IP
	PortalPort int
	Protocol   api.Protocol
	ProxyPort  int
	PublicIPs  []string // TODO: make this net.IP
	// unsupported
	// socket              proxySocket
	// timeout             time.Duration
	// stickyMaxAgeMinutes int
	// sessionAffinityType api.AffinityType
}

func (spn ServicePortName) String() string {
	return fmt.Sprintf("%s:%s", spn.NamespacedName.String(), spn.Port)
}

func ensurePortals(serviceMap map[ServicePortName]*ServiceInfo) {
	// NB: This does not remove rules that should not be present.
	for name, info := range serviceMap {
		err := openPortal(name, info)
		if err != nil {
			glog.Errorf("Failed to ensure portal for %q: %v", name, err)
		}
	}
}

func ipsEqual(lhs, rhs []string) bool {
	if len(lhs) != len(rhs) {
		return false
	}
	for i := range lhs {
		if lhs[i] != rhs[i] {
			return false
		}
	}
	return true
}

func openPortal(service ServicePortName, info *ServiceInfo) error {
	err := openOnePortal(info.PortalIP, info.PortalPort, info.Protocol, listenIP, info.ProxyPort, service)
	if err != nil {
		return err
	}
	for _, publicIP := range info.PublicIPs {
		err = openOnePortal(net.ParseIP(publicIP), info.PortalPort, info.Protocol, listenIP, info.ProxyPort, service)
		if err != nil {
			return err
		}
	}
	return nil
}

func openOnePortal(portalIP net.IP, portalPort int, protocol api.Protocol, proxyIP net.IP, proxyPort int, name ServicePortName) error {
	// Handle traffic from containers.
	args := iptablesContainerPortalArgs(portalIP, portalPort, protocol, proxyIP, proxyPort, name)
	existed, err := ipt.EnsureRule(iptables.TableNAT, iptablesContainerPortalChain, args...)
	if err != nil {
		glog.Errorf("Failed to install iptables %s rule for service %q", iptablesContainerPortalChain, name)
		return err
	}
	if !existed {
		glog.Infof("Opened iptables from-containers portal for service %q on %s %s:%d", name, protocol, portalIP, portalPort)
	}

	// Handle traffic from the host.
	args = iptablesHostPortalArgs(portalIP, portalPort, protocol, proxyIP, proxyPort, name)
	existed, err = ipt.EnsureRule(iptables.TableNAT, iptablesHostPortalChain, args...)
	if err != nil {
		glog.Errorf("Failed to install iptables %s rule for service %q", iptablesHostPortalChain, name)
		return err
	}
	if !existed {
		glog.Infof("Opened iptables from-host portal for service %q on %s %s:%d", name, protocol, portalIP, portalPort)
	}
	return nil
}

func closePortal(service ServicePortName, info *ServiceInfo) error {
	// Collect errors and report them all at the end.
	el := closeOnePortal(info.PortalIP, info.PortalPort, info.Protocol, listenIP, info.ProxyPort, service)
	for _, publicIP := range info.PublicIPs {
		el = append(el, closeOnePortal(net.ParseIP(publicIP), info.PortalPort, info.Protocol, listenIP, info.ProxyPort, service)...)
	}
	if len(el) == 0 {
		glog.Infof("Closed iptables portals for service %q", service)
	} else {
		glog.Errorf("Some errors closing iptables portals for service %q", service)
	}
	return errors.NewAggregate(el)
}

func closeOnePortal(portalIP net.IP, portalPort int, protocol api.Protocol, proxyIP net.IP, proxyPort int, name ServicePortName) []error {
	el := []error{}

	// Handle traffic from containers.
	args := iptablesContainerPortalArgs(portalIP, portalPort, protocol, proxyIP, proxyPort, name)
	if err := ipt.DeleteRule(iptables.TableNAT, iptablesContainerPortalChain, args...); err != nil {
		glog.Errorf("Failed to delete iptables %s rule for service %q", iptablesContainerPortalChain, name)
		el = append(el, err)
	}

	// Handle traffic from the host.
	args = iptablesHostPortalArgs(portalIP, portalPort, protocol, proxyIP, proxyPort, name)
	if err := ipt.DeleteRule(iptables.TableNAT, iptablesHostPortalChain, args...); err != nil {
		glog.Errorf("Failed to delete iptables %s rule for service %q", iptablesHostPortalChain, name)
		el = append(el, err)
	}

	return el
}

// See comments in the *PortalArgs() functions for some details about why we
// use two chains.
var iptablesContainerPortalChain iptables.Chain = "KUBE-PORTALS-CONTAINER"
var iptablesHostPortalChain iptables.Chain = "KUBE-PORTALS-HOST"
var iptablesOldPortalChain iptables.Chain = "KUBE-PROXY"

// Ensure that the iptables infrastructure we use is set up.  This can safely be called periodically.
func iptablesInit(ipt iptables.Interface) error {
	// TODO: There is almost certainly room for optimization here.  E.g. If
	// we knew the portal_net CIDR we could fast-track outbound packets not
	// destined for a service. There's probably more, help wanted.
	if _, err := ipt.EnsureChain(iptables.TableNAT, iptablesContainerPortalChain); err != nil {
		return err
	}
	if _, err := ipt.EnsureRule(iptables.TableNAT, iptables.ChainPrerouting, "-j", string(iptablesContainerPortalChain)); err != nil {
		return err
	}
	if _, err := ipt.EnsureChain(iptables.TableNAT, iptablesHostPortalChain); err != nil {
		return err
	}
	if _, err := ipt.EnsureRule(iptables.TableNAT, iptables.ChainOutput, "-j", string(iptablesHostPortalChain)); err != nil {
		return err
	}
	return nil
}

// Flush all of our custom iptables rules.
func iptablesFlush(ipt iptables.Interface) error {
	el := []error{}
	if err := ipt.FlushChain(iptables.TableNAT, iptablesContainerPortalChain); err != nil {
		el = append(el, err)
	}
	if err := ipt.FlushChain(iptables.TableNAT, iptablesHostPortalChain); err != nil {
		el = append(el, err)
	}
	if len(el) != 0 {
		glog.Errorf("Some errors flushing old iptables portals: %v", el)
	}
	return errors.NewAggregate(el)
}

// Used below.
var zeroIPv4 = net.ParseIP("0.0.0.0")
var localhostIPv4 = net.ParseIP("127.0.0.1")

var zeroIPv6 = net.ParseIP("::0")
var localhostIPv6 = net.ParseIP("::1")

// Build a slice of iptables args that are common to from-container and from-host portal rules.
func iptablesCommonPortalArgs(destIP net.IP, destPort int, protocol api.Protocol, service ServicePortName) []string {
	// This list needs to include all fields as they are eventually spit out
	// by iptables-save.  This is because some systems do not support the
	// 'iptables -C' arg, and so fall back on parsing iptables-save output.
	// If this does not match, it will not pass the check.  For example:
	// adding the /32 on the destination IP arg is not strictly required,
	// but causes this list to not match the final iptables-save output.
	// This is fragile and I hope one day we can stop supporting such old
	// iptables versions.
	args := []string{
		"-m", "comment",
		"--comment", service.String(),
		"-p", strings.ToLower(string(protocol)),
		"-m", strings.ToLower(string(protocol)),
		"-d", fmt.Sprintf("%s/32", destIP.String()),
		"--dport", fmt.Sprintf("%d", destPort),
	}
	return args
}

// Build a slice of iptables args for a from-container portal rule.
func iptablesContainerPortalArgs(destIP net.IP, destPort int, protocol api.Protocol, proxyIP net.IP, proxyPort int, service ServicePortName) []string {
	args := iptablesCommonPortalArgs(destIP, destPort, protocol, service)

	// This is tricky.
	//
	// If the proxy is bound (see Proxier.listenIP) to 0.0.0.0 ("any
	// interface") we want to use REDIRECT, which sends traffic to the
	// "primary address of the incoming interface" which means the container
	// bridge, if there is one.  When the response comes, it comes from that
	// same interface, so the NAT matches and the response packet is
	// correct.  This matters for UDP, since there is no per-connection port
	// number.
	//
	// The alternative would be to use DNAT, except that it doesn't work
	// (empirically):
	//   * DNAT to 127.0.0.1 = Packets just disappear - this seems to be a
	//     well-known limitation of iptables.
	//   * DNAT to eth0's IP = Response packets come from the bridge, which
	//     breaks the NAT, and makes things like DNS not accept them.  If
	//     this could be resolved, it would simplify all of this code.
	//
	// If the proxy is bound to a specific IP, then we have to use DNAT to
	// that IP.  Unlike the previous case, this works because the proxy is
	// ONLY listening on that IP, not the bridge.
	//
	// Why would anyone bind to an address that is not inclusive of
	// localhost?  Apparently some cloud environments have their public IP
	// exposed as a real network interface AND do not have firewalling.  We
	// don't want to expose everything out to the world.
	//
	// Unfortunately, I don't know of any way to listen on some (N > 1)
	// interfaces but not ALL interfaces, short of doing it manually, and
	// this is simpler than that.
	//
	// If the proxy is bound to localhost only, all of this is broken.  Not
	// allowed.
	if proxyIP.Equal(zeroIPv4) || proxyIP.Equal(zeroIPv6) {
		// TODO: Can we REDIRECT with IPv6?
		args = append(args, "-j", "REDIRECT", "--to-ports", fmt.Sprintf("%d", proxyPort))
	} else {
		// TODO: Can we DNAT with IPv6?
		args = append(args, "-j", "DNAT", "--to-destination", net.JoinHostPort(proxyIP.String(), strconv.Itoa(proxyPort)))
	}
	return args
}

// Build a slice of iptables args for a from-host portal rule.
func iptablesHostPortalArgs(destIP net.IP, destPort int, protocol api.Protocol, proxyIP net.IP, proxyPort int, service ServicePortName) []string {
	args := iptablesCommonPortalArgs(destIP, destPort, protocol, service)

	// This is tricky.
	//
	// If the proxy is bound (see Proxier.listenIP) to 0.0.0.0 ("any
	// interface") we want to do the same as from-container traffic and use
	// REDIRECT.  Except that it doesn't work (empirically).  REDIRECT on
	// localpackets sends the traffic to localhost (special case, but it is
	// documented) but the response comes from the eth0 IP (not sure why,
	// truthfully), which makes DNS unhappy.
	//
	// So we have to use DNAT.  DNAT to 127.0.0.1 can't work for the same
	// reason.
	//
	// So we do our best to find an interface that is not a loopback and
	// DNAT to that.  This works (again, empirically).
	//
	// If the proxy is bound to a specific IP, then we have to use DNAT to
	// that IP.  Unlike the previous case, this works because the proxy is
	// ONLY listening on that IP, not the bridge.
	//
	// If the proxy is bound to localhost only, this should work, but we
	// don't allow it for now.
	if proxyIP.Equal(zeroIPv4) || proxyIP.Equal(zeroIPv6) {
		proxyIP = hostIP
	}
	// TODO: Can we DNAT with IPv6?
	args = append(args, "-j", "DNAT", "--to-destination", net.JoinHostPort(proxyIP.String(), strconv.Itoa(proxyPort)))
	return args
}

func isTooManyFDsError(err error) bool {
	return strings.Contains(err.Error(), "too many open files")
}

func isClosedError(err error) bool {
	// A brief discussion about handling closed error here:
	// https://code.google.com/p/go/issues/detail?id=4373#c14
	// TODO: maybe create a stoppable TCP listener that returns a StoppedError
	return strings.HasSuffix(err.Error(), "use of closed network connection")
}
