package en

var areaUS = map[string]string{
	"program.starting":     "Starting Go-OpenBmclApi v%s (%s)",
	"error.set.cluster.id": "Please set cluster-id and cluster-secret in config.yaml before start!",
	"error.init.failed":    "Cannot init cluster: %v",

	"program.exited":                          "Program exiting with code %d",
	"error.exit.please.read.faq":              "Please read https://github.com/LiterMC/go-openbmclapi?tab=readme-ov-file#faq before report your issue",
	"warn.exit.detected.windows.open.browser": "Detected that you are in windows environment, we are helping you to open the browser",

	"info.filelist.fetching":      "Fetching file list",
	"error.filelist.fetch.failed": "Cannot fetch cluster file list: %v",

	"error.address.listen.failed": "Cannot listen address %s: %v",

	"info.cert.requesting":              "Requesting certificates, please wait ...",
	"info.cert.requested":               "Requested certificate for %s",
	"error.cert.not.set":                "No certificates was set in the config",
	"error.cert.parse.failed":           "Cannot parse certificate key pair[%d]: %v",
	"error.cert.request.failed":         "Error when requesting certificate key pair: %v",
	"error.cert.requested.parse.failed": "Cannot parse requested certificate key pair: %v",

	"info.server.public.at":         "Server public at https://%s:%d (%s) with %d certificates",
	"info.server.alternative.hosts": "Alternative hostnames:",
	"info.wait.first.sync":          "Waiting for the first sync ...",
	"error.cluster.enable.failed":   "Cannot enable cluster: %v",

	"warn.server.closing":          "Closing server ...",
	"warn.server.closed":           "Server closed.",
	"info.cluster.disabling":       "Disabling cluster ...",
	"error.cluster.disable.failed": "Cluster disable failed: %v",
	"warn.cluster.disabled":        "Cluster disabled",
	"warn.httpserver.closing":      "Closing HTTP server ...",

	"info.check.start":                "Start checking files for %s, heavy = %v",
	"info.check.done":                 "File check finished for %s, missing %d files",
	"hint.check.checking":             "> Checking ",
	"warn.check.modified.size":        "Found modified file: size of %q is %d, expect %d",
	"warn.check.modified.hash":        "Found modified file: hash of %q is %s, expect %s",
	"error.check.unknown.hash.method": "Unknown hash method for %q",
	"error.check.open.failed":         "Cannot open %q: %v",
	"error.check.hash.failed":         "Cannot calculate hash for %s: %v",

	"info.sync.prepare":          "Preparing to sync files, length of filelist is %d ...",
	"hint.sync.start":            "Starting sync files, count: %d, bytes: %s",
	"hint.sync.done":             "All files were synchronized, use time: %v, %s/s",
	"info.sync.none":             "All files were synchronized",
	"warn.sync.interrupted":      "File sync interrupted",
	"info.sync.config":           "Sync config: %#v",
	"hint.sync.total":            "Total: ",
	"hint.sync.downloading":      "> Downloading ",
	"info.sync.downloaded":       "Downloaded %s [%s] %.2f%%",
	"error.sync.download.failed": "Download error %s:\n\t%s",
	"error.sync.create.failed":   "Cannot create %s/%s: %v",

	"info.gc.start":       "Starting garbage collector for %s",
	"info.gc.done":        "Garbage collect finished for %s",
	"warn.gc.interrupted": "Garbage collector interrupted at %s",
	"info.gc.found":       "Found outdated file %s",
	"error.gc.error":      "Garbage collector error: %v",

	"error.config.read.failed":           "Cannot read config: %v",
	"error.config.encode.failed":         "Cannot encode config: %v",
	"error.config.write.failed":          "Cannot write config: %v",
	"error.config.not.exists":            "Config file not exists, creating one",
	"error.config.created":               "Config file created, please edit it and start the program again",
	"error.config.parse.failed":          "Cannot parse config: %v",
	"error.config.alias.user.not.exists": "WebDav alias user %q does not exists",
}
