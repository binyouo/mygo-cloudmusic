#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
dev_dir=${CLOUDMUSIC_IOS_DEV_DIR:-$(dirname -- "$root")/mygo-ios-dev}
mkdir -p "$dev_dir"
dev_dir=$(CDPATH='' cd -- "$dev_dir" && pwd -P)
mkdir -p "$dev_dir/mod" "$dev_dir/cache" "$dev_dir/tmp"
export GOMODCACHE="$dev_dir/mod"
export GOCACHE="$dev_dir/cache"
export GOTMPDIR="$dev_dir/tmp"
export TMPDIR="$dev_dir/tmp"
if [ -x "$dev_dir/toolchain/bin/go" ]; then
    export PATH="$dev_dir/toolchain/bin:$PATH"
fi
if [ -z "${DEVELOPER_DIR:-}" ] && [ -d "$dev_dir/Xcode.app/Contents/Developer" ]; then
    export DEVELOPER_DIR="$dev_dir/Xcode.app/Contents/Developer"
fi

cd "$root"
app_goflags=${GOFLAGS:-}
export GOFLAGS="${app_goflags:+$app_goflags }-modfile=$root/go.ios.mod"

# In this MyGo snapshot iOS uses Metal, but the custom shader compiler
# selects GLSL on iOS. Use the same compiler as macOS and regenerate the
# built-in library when it predates shader.metal, in a cached module copy.
prepare_mygo_metal() (
    module=github.com/egoist/mygo
    go mod download "$module"
    upstream=$(go list -m -f '{{.Dir}}' "$module")
    metal=internal/gpu/metal
    library="$metal/shaderlib_ios_gen.go"
    if [ ! -f "$upstream/$library" ]; then
        exit 0
    fi
    digest=$(shasum -a 256 "$upstream/$metal/shader.metal")
    digest=${digest%% *}
    rebuild_library=false
    if ! grep -Fxq "const shaderLibrarySum = \"$digest\"" "$upstream/$library"; then
        rebuild_library=true
    fi
    shader_tags=false
    if grep -Fxq '//go:build !ios' "$upstream/ui/shader/code_darwin.go" &&
        grep -Fxq '//go:build !(darwin && !ios) && !windows' "$upstream/ui/shader/code_other.go"; then
        shader_tags=true
    fi
    if [ "$rebuild_library" = false ] && [ "$shader_tags" = false ]; then
        exit 0
    fi

    cached="$dev_dir/$(basename -- "$upstream")-metal-$digest-ios-msl"
    lockf -k "$cached.lock" sh -eu -c '
        if [ -d "$2" ]; then
            exit 0
        fi
        temporary=$(mktemp -d "$2.XXXXXX")
        trap '\''rm -rf -- "$temporary"'\'' EXIT
        cp -R "$1" "$temporary/mygo"
        cd "$temporary/mygo"
        chmod -R u+w .
        if [ "$4" = true ]; then
            sed -i "" "1d" ui/shader/code_darwin.go
            sed -i "" "1s|.*|//go:build !darwin \&\& !windows|" ui/shader/code_other.go
        fi
        if [ "$5" = true ]; then
            GOFLAGS= GOWORK=off go generate "./$3" >&2
        fi
        mv "$temporary/mygo" "$2"
    ' sh "$upstream" "$cached" "$metal" "$shader_tags" "$rebuild_library"
    printf '%s\n' "$cached"
)

prepare_rubberband() (
    version=4.0.0
    source="rubberband-$version"
    sha=af050313ee63bc18b35b2e064e5dce05b276aaf6d1aa2b8a82ced1fe2f8028e9
    if [ -f "$root/build/$source/single/RubberBandSingle.cpp" ]; then
        exit 0
    fi
    mkdir -p "$root/build" "$dev_dir/rubberband"
    archive="$dev_dir/rubberband/$source.tar.bz2"
    temporary=$(mktemp -d "$root/build/.rubberband.XXXXXX")
    trap 'rm -rf -- "$temporary"' EXIT
    if [ ! -f "$archive" ]; then
        curl --fail --location --silent --show-error --retry 2 \
            "https://breakfastquay.com/files/releases/$source.tar.bz2" \
            --output "$temporary/$source.tar.bz2"
        printf '%s  %s\n' "$sha" "$temporary/$source.tar.bz2" | shasum -a 256 -c -
        mv "$temporary/$source.tar.bz2" "$archive"
    fi
    printf '%s  %s\n' "$sha" "$archive" | shasum -a 256 -c -
    tar -xjf "$archive" -C "$temporary" "$source/single" "$source/src" "$source/rubberband"
    mv -n "$temporary/$source" "$root/build/"
)

prepare_soxr() (
    source=soxr-1.0.0
    digest=e07ee6c1d659bc6957034f4800c60cb8b98de798823e34d2a2bba1caa85a4509
    mkdir -p "$root/build" "$dev_dir/soxr"
    temporary=$(mktemp -d "$root/build/.soxr.XXXXXX")
    trap 'rm -rf -- "$temporary"' EXIT
    archive="$dev_dir/soxr/$source.tar.gz"
    if [ ! -f "$root/build/$source/libsoxr/src/soxr.h" ]; then
        if [ ! -f "$archive" ]; then
            curl --fail --location --silent --show-error --retry 2 \
                'https://files.pythonhosted.org/packages/42/7e/f4b461944662ad75036df65277d6130f9411002bfb79e9df7dff40a31db9/soxr-1.0.0.tar.gz' \
                --output "$temporary/$source.tar.gz"
            printf '%s  %s\n' "$digest" "$temporary/$source.tar.gz" | shasum -a 256 -c -
            mv "$temporary/$source.tar.gz" "$archive"
        fi
        printf '%s  %s\n' "$digest" "$archive" | shasum -a 256 -c -
        tar -xzf "$archive" -C "$temporary" "$source/libsoxr/src"
        mv -n "$temporary/$source" "$root/build/"
    fi
    # Each libsoxr source is a separate C compilation unit, using the same
    # SDK and target as the rest of the Go archive.
    for unit in soxr data-io dbesi0 filter fft4g64 cr cr32 fft4g32 cr64; do
        filename="soxr_source_$unit.c"
        printf '%s\n\n#include "%s.c"\n' \
            '//go:build cgo && (ios || (darwin && cloudmusic_static_rubberband))' "$unit" > "$temporary/$filename"
        if ! cmp -s "$temporary/$filename" "$root/internal/player/$filename"; then
            mv "$temporary/$filename" "$root/internal/player/$filename"
        fi
    done
)

prepare_coreml() (
    destination="$root/resources/automix"
    models="${CLOUDMUSIC_COREML_DIR:-$dev_dir/coreml/verified-models}"
    if [ -z "${CLOUDMUSIC_COREML_DIR:-}" ] && [ ! -e "$models" ] && [ -d "$destination" ]; then
        models="$destination"
    fi

    require_file() {
        if [ ! -f "$1" ] || [ ! -s "$1" ]; then
            printf 'Missing or empty AutoMix resource: %s\n' "$1" >&2
            printf 'Set CLOUDMUSIC_COREML_DIR to the output of scripts/smart_fades/export_coreml.py.\n' >&2
            exit 1
        fi
    }

    mkdir -p "$root/resources" "$root/build"
    temporary=$(mktemp -d "$root/resources/.automix.XXXXXX")
    trap 'rm -rf -- "$temporary/automix"; rmdir "$temporary" 2>/dev/null || :' EXIT

    staged="$temporary/automix"
    for model in BeatThis FireRed SKeyFeatures SKeyClassify; do
        package="$model.mlpackage"
        manifest="$models/$package/Manifest.json"
        base=Data/com.apple.CoreML
        require_file "$manifest"
        require_file "$models/$package/$base/model.mlmodel"
        require_file "$models/$package/$base/weights/weight.bin"
        model_id=$(plutil -extract rootModelIdentifier raw -expect string -o - "$manifest") &&
            model_path=$(plutil -extract "itemInfoEntries.$model_id.path" raw -expect string -o - "$manifest") || {
                printf 'Invalid AutoMix model manifest: %s\n' "$manifest" >&2
                exit 1
            }
        if [ "$model_path" != com.apple.CoreML/model.mlmodel ]; then
            printf 'Unexpected model path in %s: %s\n' "$manifest" "$model_path" >&2
            exit 1
        fi
        mkdir -p "$staged/$package/$base/weights"
        cp "$manifest" "$staged/$package/"
        cp "$models/$package/$base/model.mlmodel" "$staged/$package/$base/"
        cp "$models/$package/$base/weights/weight.bin" "$staged/$package/$base/weights/"
    done
    stats="$models/FireRedCMVN.json"
    require_file "$stats"
    for key in means inverseStd; do
        if ! count=$(plutil -extract "$key" raw -expect array -o - "$stats") || [ "$count" != 80 ]; then
            printf 'Expected 80 values for %s in %s\n' "$key" "$stats" >&2
            exit 1
        fi
    done
    cp "$stats" "$staged/"

    # Keep simultaneous invocations from moving a second copy into automix/.
    lockf -k "$root/build/.automix.lock" sh -eu -c '
        staged=$1 destination=$2 previous=$3
        if diff -qr "$staged" "$destination" >/dev/null 2>&1; then
            exit 0
        fi
        if [ -e "$destination" ]; then
            mv "$destination" "$previous"
        fi
        if ! mv "$staged" "$destination"; then
            if [ -e "$previous" ]; then
                mv "$previous" "$destination"
            fi
            exit 1
        fi
        rm -rf -- "$previous"
    ' sh "$staged" "$destination" "$temporary/previous"
    printf 'Prepared AutoMix models at %s\n' "$destination"
)

preview_data="$root/build/ios-preview"
preview_bundle="$preview_data/MyGo Cloud Music iOS Preview.app"
preview_executable=Contents/MacOS/cloudmusic-ios-preview

build_preview() (
    prepare_rubberband
    prepare_soxr
    export CLANG_MODULE_CACHE_PATH="$dev_dir/cache/clang-modules"
    mkdir -p "$preview_data"
    temporary=$(mktemp -d "$preview_data/.build.XXXXXX")
    staged="$temporary/$(basename -- "$preview_bundle")"
    trap 'rm -rf -- "$staged"; rmdir "$temporary" 2>/dev/null || :' EXIT

    mkdir -p "$(dirname -- "$staged/$preview_executable")"
    cat >"$staged/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
    <key>CFBundleIdentifier</key><string>dev.egoist.cloudmusic.iospreview</string>
    <key>CFBundleName</key><string>Cloud Music iOS Preview</string>
    <key>CFBundleExecutable</key><string>cloudmusic-ios-preview</string>
    <key>CFBundlePackageType</key><string>APPL</string>
</dict></plist>
PLIST
    CGO_ENABLED=1 go build -tags cloudmusic_static_rubberband \
        -ldflags "-X 'netease-music-mygo/internal/app.phonePreviewRoot=$preview_data' -X 'netease-music-mygo/internal/player.neuralCacheBase=$preview_data/cache'" \
        -o "$staged/$preview_executable" .
    codesign --force --sign - "$staged"
    codesign --verify --strict "$staged"

    # Keep the last verified app available until the new bundle is ready.
    lockf -k "$preview_data/.build.lock" sh -eu -c '
        staged=$1 bundle=$2 previous=$3
        if [ -e "$bundle" ]; then
            mv "$bundle" "$previous"
        fi
        if ! mv "$staged" "$bundle"; then
            if [ -e "$previous" ]; then
                mv "$previous" "$bundle"
            fi
            exit 1
        fi
        if ! codesign --verify --strict "$bundle"; then
            mv "$bundle" "$staged"
            if [ -e "$previous" ]; then
                mv "$previous" "$bundle"
            fi
            exit 1
        fi
        rm -rf -- "$previous"
    ' sh "$staged" "$preview_bundle" "$temporary/previous.app"
    printf '%s\n' "$preview_bundle"
)

case "${1:-}" in
    prepare-rubberband)
        prepare_rubberband
        exit
        ;;
    prepare-soxr)
        prepare_soxr
        exit
        ;;
    prepare-coreml)
        prepare_coreml
        exit
        ;;
    preview-build)
        build_preview
        exit
        ;;
    preview)
        build_preview
        exec "$preview_bundle/$preview_executable"
        ;;
    build|dev)
        command=$1
        shift
        case "${1:-}" in
            -h|--help|-help) exec go tool mygo "$command" "$@" ;;
        esac
        if ! xcrun --find xcodebuild >/dev/null 2>&1; then
            printf 'Full Xcode is required for iOS builds. Put Xcode.app in %s or set DEVELOPER_DIR to its Contents/Developer directory.\n' "$dev_dir" >&2
            exit 1
        fi
        platform=ios
        if [ "$command" = build ]; then
            platform=ios/simulator
        fi
        set -- "$command" -platform "$platform" "$@"
        prepare_coreml
        prepare_rubberband
        prepare_soxr
        # Compile C++ with the same device or simulator SDK that MyGo passes to CC.
        export CXX="'$root/scripts/mygo-ios-cxx.sh'"
        export CLANG_MODULE_CACHE_PATH="$dev_dir/cache/clang-modules"
        metal_module=$(prepare_mygo_metal)
        if [ -n "$metal_module" ]; then
            temporary=$(mktemp -d "$GOTMPDIR/mygo-ios-mod.XXXXXX")
            trap 'rm -rf -- "$temporary"' EXIT
            cp go.ios.mod "$temporary/go.ios.mod"
            cp go.ios.sum "$temporary/go.ios.sum"
            export GOFLAGS="${app_goflags:+$app_goflags }-modfile=$temporary/go.ios.mod"
            go mod edit -replace "github.com/egoist/mygo=$metal_module"
            go tool mygo "$@"
            exit
        fi
        ;;
esac

exec go tool mygo "$@"
