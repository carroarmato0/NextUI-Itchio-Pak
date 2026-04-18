.PHONY: test test-coverage build-native build-tg5040 build-tg5050 build-my355 build-all release deploy deploy-sd debug-logs debug-push debug-run clean

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

debug-logs:
	./scripts/debug.sh logs

debug-push:
	./scripts/debug.sh push

debug-run:
	./scripts/debug.sh run

clean:
	rm -rf bin/ dist/ coverage.out coverage.html debug-cache/
