package main

import (
	"flag"
	"time"
)

func parseFlags() options {
	var o options
	flag.StringVar(&o.csvPath, "csv", "", "CSV file whose first column holds AP IP addresses")
	flag.StringVar(&o.out, "out", "", "write results here (.csv or .json)")
	flag.StringVar(&o.user, "user", "", "AP username")
	flag.StringVar(&o.pass, "pass", "", "AP password")
	flag.BoolVar(&o.askPass, "ask-pass", false, "prompt for the AP password instead of passing it on the command line")
	flag.StringVar(&o.passEnv, "pass-env", "", "read the AP password from this environment variable")
	flag.BoolVar(&o.alsoDefault, "default", false, "also try the factory-default super/sp-admin login")
	flag.IntVar(&o.concurrency, "c", 25, "how many APs to work at once")

	flag.BoolVar(&o.fw, "fw", false, "push a firmware update")
	flag.StringVar(&o.fwProto, "fw-proto", "http", "firmware server protocol: http, ftp or tftp")
	flag.StringVar(&o.fwHost, "fw-host", "", "firmware server address")
	flag.StringVar(&o.fwPort, "fw-port", "80", "firmware server port")
	flag.StringVar(&o.fwUser, "fw-user", "", "firmware server username")
	flag.StringVar(&o.fwPass, "fw-pass", "", "firmware server password")
	flag.StringVar(&o.fwFile, "fw-file", "", "firmware filename; %M is replaced with the detected model")

	flag.StringVar(&o.serveDir, "serve", "", "host the firmware images from this directory over the tool's own HTTP server (sets -fw-proto/-fw-host/-fw-port)")
	flag.IntVar(&o.servePort, "serve-port", 8080, "port for the built-in image server; 0 picks a free one")
	flag.StringVar(&o.serveIP, "serve-ip", "", "address the APs should fetch from (default: whichever local address routes to them)")
	flag.DurationVar(&o.serveWait, "serve-wait", 30*time.Minute, "how long to keep serving after the pushes are started")
	flag.DurationVar(&o.fwWait, "fw-wait", 0, "after starting the update, hold the session open this long to capture the AP's progress output")
	flag.BoolVar(&o.factory, "factory", false, "reset the AP to factory defaults (implies -reboot; the reset is inert until then)")
	flag.BoolVar(&o.reboot, "reboot", false, "reboot the AP when finished")
	flag.StringVar(&o.command, "cmd", "", "run an arbitrary AP CLI command")

	flag.StringVar(&o.deadOut, "dead", "", "write the addresses that did not answer to this file, one per line (re-feedable as -csv)")
	flag.StringVar(&o.probe, "probe", "icmp", "reachability check before SSH: icmp, tcp, both or none")
	flag.DurationVar(&o.pingTimeout, "ping-timeout", 1500*time.Millisecond, "per-attempt reachability timeout")
	flag.IntVar(&o.pingRetries, "ping-retries", 1, "extra attempts for addresses that stayed silent")
	flag.IntVar(&o.pingConcurrency, "pc", 256, "how many addresses to probe at once")

	flag.StringVar(&o.sshPort, "port", "22", "SSH port")
	flag.DurationVar(&o.timeout, "timeout", 8*time.Second, "per-step timeout")
	flag.BoolVar(&o.legacy, "legacy", true, "allow the SHA-1/CBC algorithms old ZoneFlex firmware needs")
	flag.BoolVar(&o.verbose, "v", false, "dump the full session transcript for each AP")
	flag.BoolVar(&o.showVers, "version", false, "print version and exit")
	flag.Parse()
	return o
}
