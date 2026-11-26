.PHONY : dev build
.DEFAULT_GOAL := dev # -> just make

dev:
	@set -a && . ./.env && set +a && \
		go run \
			-ldflags "-X 'yamaclient/lib/info/version.version=dev' \
			-X 'yamaclient/lib/info/version.source=local'" \
			. $(ARGS)