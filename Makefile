.PHONY: test test-coverage build-native build-nextui build-muos build-all \
        build-nextui-tg5040 build-nextui-tg5050 build-nextui-my355 build-muos-arm64 \
        build-tg5040 build-tg5050 build-my355 \
        release deploy deploy-sd deploy-adb debug-logs debug-push debug-run clean

test:
	./scripts/test.sh

test-coverage:
	./scripts/test.sh --coverage

build-native:
	./scripts/build.sh native

# Per-firmware
build-nextui:
	./scripts/build.sh nextui

build-muos:
	./scripts/build.sh muos

# Per-target
build-nextui-tg5040:
	./scripts/build.sh nextui/tg5040

build-nextui-tg5050:
	./scripts/build.sh nextui/tg5050

build-nextui-my355:
	./scripts/build.sh nextui/my355

build-muos-arm64:
	./scripts/build.sh muos/arm64

# Deprecated aliases from before firmware and device were separated.
# Remove after one release.
build-tg5040: build-nextui-tg5040
build-tg5050: build-nextui-tg5050
build-my355: build-nextui-my355

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

# Image names come from scripts/targets.sh so adding a toolchain there is
# enough — this list never needs editing again.  The itchio-pak-* names are the
# pre-rename images; removing them too keeps upgrades from leaving junk behind.
clean:
	rm -rf bin/ dist/ coverage.out coverage.html debug-cache/
	rm -f lib/*/*.so*
	@RUNTIME=$$(command -v podman >/dev/null 2>&1 && echo podman || echo docker); \
	IMAGES="itchio-dev $$(. ./scripts/targets.sh; all_toolchains | sed 's/^/itchio-toolchain-/' | tr '\n' ' ')"; \
	LEGACY="itchio-pak-dev itchio-pak-tg5040-dev itchio-pak-tg5050-dev itchio-pak-my355-dev"; \
	$$RUNTIME rmi $$IMAGES $$LEGACY 2>/dev/null || true
