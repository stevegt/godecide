all: godecide

godecide: godecide.go fin/fin.go examples/example.yaml
	# Purely-static compilation avoids a problem with older libm and
	# glibc versions on target hosts.  Without these compilation
	# flags, the github.com/goccy/go-graphviz package will cause a
	# dependency on shared libraries.
	go build --ldflags '-linkmode external -extldflags "-static"'
