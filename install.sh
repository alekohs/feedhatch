#!/bin/bash
# Installs the latest feedhatch release the way that fits this system.
#   curl -fsSL https://raw.githubusercontent.com/alekohs/feedhatch/main/install.sh | bash
#   bash install.sh 0.2.0   # a specific release instead of the latest
set -euo pipefail

repo='alekohs/feedhatch'

die() {
  echo "$*" >&2
  exit 1
}

need() {
  for command in "$@"; do
    command -v "$command" >/dev/null || die "Missing $command; install it and run this again."
  done
}

# method picks the install method from uname -s and the os-release ID and ID_LIKE values.
method() {
  local kernel=$1 ids=" $2 $3 "
  if [[ $kernel == Darwin ]]; then
    echo macos
  elif [[ $ids == *" arch "* ]]; then
    echo arch
  elif [[ $ids == *" debian "* || $ids == *" ubuntu "* ]]; then
    echo deb
  elif [[ $ids == *" fedora "* || $ids == *" rhel "* || $ids == *" centos "* || $ids == *suse* ]]; then
    echo rpm
  else
    echo tarball
  fi
}

latest_version() {
  curl -fsSL "https://api.github.com/repos/$repo/releases/latest" |
    sed -n 's/.*"tag_name": *"v\{0,1\}\([^"]*\)".*/\1/p'
}

# download fetches a release asset into the current directory and checks it against checksums.txt.
download() {
  local asset=$1 base="https://github.com/$repo/releases/download/v$version"
  curl -fsSL "$base/$asset" -o "$asset" || die "Could not download $asset from release v$version."
  curl -fsSL "$base/checksums.txt" -o checksums.txt || die "Could not download checksums.txt from release v$version."
  local sum
  if command -v sha256sum >/dev/null; then
    sum=$(sha256sum "$asset")
  else
    sum=$(shasum -a 256 "$asset")
  fi
  grep -qxF "${sum%% *}  $asset" checksums.txt || die "Checksum mismatch for $asset; nothing was installed."
}

install_arch() {
  [[ $EUID != 0 ]] || die 'Run this as your normal user; makepkg refuses to build as root and asks for sudo itself.'
  need makepkg pacman git
  # Run from a checkout, use the PKGBUILD next to this script; piped through curl, fetch it.
  local script=${BASH_SOURCE[0]:-}
  if [[ -n $script && -f $script && -f $(dirname "$script")/packaging/arch/PKGBUILD ]]; then
    cp "$(dirname "$script")/packaging/arch/PKGBUILD" PKGBUILD
  else
    curl -fsSL "https://raw.githubusercontent.com/$repo/main/packaging/arch/PKGBUILD" -o PKGBUILD
  fi
  sed -i "s/^pkgver=.*/pkgver=$version/" PKGBUILD
  # Piped through curl, stdin is the script itself, so pacman's prompts would read
  # end-of-input and abort; answer them from the terminal instead.
  if [[ -t 0 ]]; then
    makepkg --syncdeps --install --clean
  elif (exec </dev/tty) 2>/dev/null; then
    makepkg --syncdeps --install --clean </dev/tty
  else
    makepkg --syncdeps --install --clean --noconfirm
  fi
}

install_deb() {
  local asset="feedhatch_${version}_${goarch}.deb"
  download "$asset"
  $sudo apt-get install -y "./$asset"
}

install_rpm() {
  local asset="feedhatch-${version}-1.${rpmarch}.rpm"
  download "$asset"
  if command -v dnf >/dev/null; then
    $sudo dnf install -y "./$asset"
  elif command -v zypper >/dev/null; then
    $sudo zypper --non-interactive install --allow-unsigned-rpm "./$asset"
  else
    $sudo rpm -U "./$asset"
  fi
}

install_macos() {
  local asset="feedhatch_${version}_darwin_${goarch}.tar.gz"
  download "$asset"
  tar -xzf "$asset"
  $sudo install -m 755 feedhatch /usr/local/bin/feedhatch
}

# install_tarball puts everything under the home directory, for Linux systems without a package.
install_tarball() {
  local asset="feedhatch_${version}_linux_${goarch}.tar.gz"
  download "$asset"
  tar -xzf "$asset"
  install -Dm755 feedhatch "$HOME/.local/bin/feedhatch"
  install -Dm644 packaging/linux/feedhatch.desktop "$HOME/.local/share/applications/feedhatch.desktop"
  install -Dm644 packaging/linux/feedhatch.svg "$HOME/.local/share/icons/hicolor/scalable/apps/feedhatch.svg"
  mkdir -p "$HOME/.config/systemd/user"
  sed 's|^ExecStart=/usr/bin/|ExecStart=%h/.local/bin/|' packaging/linux/feedhatch.service \
    >"$HOME/.config/systemd/user/feedhatch.service"
  case ":$PATH:" in
    *":$HOME/.local/bin:"*) ;;
    *) echo "Add ~/.local/bin to your PATH to start feedhatch by name." >&2 ;;
  esac
}

main() {
  need curl uname tar
  version=${1:-}
  if [[ -z $version ]]; then
    version=$(latest_version)
    [[ -n $version ]] || die "Could not find a published release of $repo."
  fi

  case $(uname -m) in
    x86_64 | amd64) goarch=amd64 rpmarch=x86_64 ;;
    aarch64 | arm64) goarch=arm64 rpmarch=aarch64 ;;
    *) die "Unsupported CPU architecture: $(uname -m)" ;;
  esac
  sudo=''
  if [[ $EUID != 0 ]]; then
    sudo=sudo
  fi

  local id='' id_like=''
  if [[ -r /etc/os-release ]]; then
    id=$(. /etc/os-release && echo "${ID:-}")
    id_like=$(. /etc/os-release && echo "${ID_LIKE:-}")
  fi
  local how
  how=$(method "$(uname -s)" "$id" "$id_like")

  work=$(mktemp -d)
  trap 'rm -rf "$work"' EXIT
  cd "$work"

  echo "Installing feedhatch $version ($how)"
  "install_$how"

  echo
  echo "feedhatch $version is installed. Start it with \`feedhatch\` or from your app launcher."
  if [[ $how != macos ]]; then
    echo 'To keep fetching while the TUI is closed: systemctl --user enable --now feedhatch'
  fi
}

# Sourcing the script (for tests) only defines the functions.
if [[ ${BASH_SOURCE[0]:-$0} == "$0" ]]; then
  main "$@"
fi
