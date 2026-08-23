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
	flag.BoolVar(&o.alsoDefault, "default", false, "also try the factory-default super/sp-admin login")
	flag.IntVar(&o.concurrency, "c", 25, "how many APs to work at once")

	flag.BoolVar(&o.fw, "fw", false, "push a firmware update")
	flag.StringVar(&o.fwProto, "fw-proto", "http", "firmware server protocol: http, ftp or tftp")
	flag.StringVar(&o.fwHost, "fw-host", "", "firmware server address")
	flag.StringVar(&o.fwPort, "fw-port", "80", "firmware server port")
	flag.StringVar(&o.fwUser, "fw-user", "", "firmware server username")
	flag.StringVar(&o.fwPass, "fw-pass", "", "firmware server password")
	flag.StringVar(&o.fwFile, "fw-file", "", "firmware filename; %M is replaced with the detected model")

	flag.BoolVar(&o.factory, "factory", false, "reset the AP to factory defaults")
	flag.BoolVar(&o.reboot, "reboot", false, "reboot the AP when finished")
	flag.StringVar(&o.command, "cmd", "", "run an arbitrary AP CLI command")

	flag.StringVar(&o.sshPort, "port", "22", "SSH port")
	flag.DurationVar(&o.timeout, "timeout", 8*time.Second, "per-step timeout")
	flag.BoolVar(&o.legacy, "legacy", true, "allow the SHA-1/CBC algorithms old ZoneFlex firmware needs")
	flag.BoolVar(&o.verbose, "v", false, "dump the full session transcript for each AP")
	flag.BoolVar(&o.showVers, "version", false, "print version and exit")
	flag.Parse()
	return o
}
