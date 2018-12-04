# this target should not be necessary: Go 1.11 will download things on demand.
# however, this is useful in Circle-CI as the dependencies are already cached
# this will create the `vendor` from that cache and copy it to the `docker build`
_vendor-download:
	@echo ">>> Downloading vendors"
	$(GO) mod vendor

