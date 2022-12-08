build-gopherjs:
	gopherjs build ws2s-gopherjs/main.go -o ws2s-gopherjs/ws2s.js

dev-run-lib-server-full:
	cd example-ws2s-gopherjs && python -m http.server 8002

dev-run-echo-server:
	python3 example-ws2s-gopherjs/simple-tcp-echo-server.py 127.0.0.1 9001

dev-run-ws2sd:
	python3 -m ws2s.ws2sd run

dev-fix-home-ws2sd-config-allow-localhost:
	jq --argjson new true '.allowConnectToServerLocal = true' ~/.ws2s/config.json > /tmp/config.new
	cp /tmp/config.new ~/.ws2s/config.json
