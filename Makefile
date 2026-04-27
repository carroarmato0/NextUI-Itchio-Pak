.PHONY: test test-coverage build-native build-tg5040 build-tg5050 build-my355 build-all release deploy deploy-sd deploy-adb debug-logs debug-push debug-run clean

test:
	./scripts/test.sh

test-coverage:
	./scripts/test.sh --coverage

build-native:
	./scripts/build.sh native

build-tg5040:
	./scripts/build.sh tg5040

build-tg5050:
	./scripts/build.sh tg5050

build-my355:
	./scripts/build.sh my355

build-all:
	./scripts/build.sh all

release:
	./scripts/release.sh

deploy:
	./scripts/deploy.sh

deploy-sd:
	./scripts/deploy.sh $(SD)

deploy-adb:
	./scripts/deploy.sh

debug-logs:
	./scripts/debug.sh logs

debug-push:
	./scripts/debug.sh push

debug-run:
	./scripts/debug.sh run

clean:
	rm -rf bin/ dist/ lib/ coverage.out coverage.html debug-cache/
	@RUNTIME=$$(command -v podman >/dev/null 2>&1 && echo podman || echo docker); \
	$$RUNTIME rmi itchio-pak-dev itchio-pak-tg5040-dev itchio-pak-tg5050-dev itchio-pak-my355-dev 2>/dev/null || true
