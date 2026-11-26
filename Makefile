.PHONY : dev-client
.DEFAULT_GOAL := dev-client # -> just make

dev-client:
	@make -C client/ dev
