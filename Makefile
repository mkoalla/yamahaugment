.PHONY : dev-client client-init
.DEFAULT_GOAL := dev-client # -> just make

client-init:
	@mkdir -p client-data && \
		ln -srf client-data client/data && \
		touch client/settings.toml

dev-client: client-init
	@make -C client/ dev
