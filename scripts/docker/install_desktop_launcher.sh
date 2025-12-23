#!/usr/bin/env bash
set -euo pipefail

home="${HOME:-/home/$(id -un)}"
desktop_dir="${XDG_DESKTOP_DIR:-${home}/Desktop}"
bin_dir="${home}/.local/bin"
icons_dir="${home}/.local/share/icons"
apps_dir="${home}/.local/share/applications"

mkdir -p "${desktop_dir}" "${bin_dir}" "${icons_dir}" "${apps_dir}"

root_dir="$(cd "$(dirname "$0")/../.." && pwd)"

wrapper="${bin_dir}/pterminal-docker"
cat >"${wrapper}" <<EOF
#!/usr/bin/env bash
set -euo pipefail
exec "${root_dir}/scripts/docker/run_image.sh"
EOF
chmod +x "${wrapper}"

# Install icon (SVG)
icon_src="${root_dir}/packaging/pterminal.svg"
icon_dst="${icons_dir}/pterminal.svg"
if [[ -f "${icon_src}" ]]; then
  cp -f "${icon_src}" "${icon_dst}"
fi

desktop_file="${desktop_dir}/pTerminal (Docker).desktop"
cat >"${desktop_file}" <<EOF
[Desktop Entry]
Type=Application
Name=pTerminal (Docker)
Comment=Run pTerminal via Docker
Exec=${wrapper}
Terminal=false
Categories=Network;TerminalEmulator;
Icon=${icon_dst}
EOF

chmod +x "${desktop_file}"

echo "Installed launcher:"
echo "  ${desktop_file}"
echo "Wrapper:"
echo "  ${wrapper}"
