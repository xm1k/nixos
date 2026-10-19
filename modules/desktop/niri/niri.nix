{ config, pkgs, lib, ... }:

let
  niri-shift-workspaces-down = pkgs.writeShellScriptBin "niri-shift-workspaces-down" ''
    wins=$(niri msg --json windows)
    ws_data=$(niri msg --json workspaces)

    fid=$(echo "$wins" | ${pkgs.jq}/bin/jq '[.[] | select(.is_focused)][0].id')
    ws_map=$(echo "$ws_data" | ${pkgs.jq}/bin/jq '[.[] | {key: (.id|tostring), value: .idx}] | from_entries')

    # Unique non-focused tiled columns, sorted by (ws_idx ASC, col_idx ASC).
    # Ascending order ensures the focused workspace is never emptied during moves
    # (focused column remains), so workspace indices stay stable throughout.
    moves=$(echo "$wins" | ${pkgs.jq}/bin/jq -r \
      --argjson fid "$fid" \
      --argjson wsmap "$ws_map" '
      [ .[]
      | select(
          .id != $fid
          and (.is_floating // false) == false
          and .layout.pos_in_scrolling_layout != null
        )
      | { id,
          ws_idx: ($wsmap[(.workspace_id|tostring)] // 999),
          col:    .layout.pos_in_scrolling_layout[0] }
      ]
      | group_by([.ws_idx, .col])
      | map(.[0])
      | sort_by([.ws_idx, .col])
      | .[] | "\(.id) \(.ws_idx + 1)"
    ')

    while IFS=" " read -r win_id target; do
      [ -z "$win_id" ] && continue
      niri msg action focus-window --id "$win_id"
      niri msg action move-column-to-workspace "$target"
    done <<< "$moves"

    niri msg action focus-window --id "$fid"
  '';
in
{
  imports = [./cursor.nix];

  home.packages = with pkgs; [
		xwayland
		xwayland-satellite
		xorg.xhost
		swww
    niri-shift-workspaces-down
  ];
  
  home.file.".config/niri/config.kdl" = {
    source = ./config.kdl;
    force  = true;
  };

}
