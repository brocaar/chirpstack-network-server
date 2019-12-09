build: static/img/graphs
	@echo "Building documentation"
	@hugo

clean:
	@rm -rf public
	@rm -rf static/img/graphs

static/img/graphs:
	@mkdir -p static/img/graphs/backends
	@dot -Tpng -o static/img/graphs/backends/mqtt.png graphs/backends/mqtt.dot
	@dot -Tpng -o static/img/graphs/backends/gcp_pub_sub.png graphs/backends/gcp_pub_sub.dot
	@dot -Tpng -o static/img/graphs/backends/azure_iot_hub.png graphs/backends/azure_iot_hub.dot
