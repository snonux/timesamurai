#!/usr/bin/env fish

set -l fish_config_dir (set -q XDG_CONFIG_HOME; and echo "$XDG_CONFIG_HOME/fish"; or echo "$HOME/.config/fish")
set -l fish_conf_d "$fish_config_dir/conf.d"
set -l integration_file "$fish_conf_d/timr.fish"

echo "Installing timr fish shell integration..."

# Create fish config directory if it doesn't exist
if not test -d "$fish_conf_d"
    mkdir -p "$fish_conf_d"
    echo "Created directory: $fish_conf_d"
end

# Write the integration file
echo "\
function timr_prompt -d \"Display timr timr_status in the prompt\"
    if command -v timr >/dev/null
        set -l timr_status (timr prompt)
        if test -n \"\$timr_status\"
            set -l icon (string sub -l 1 -- \"\$timr_status\")
            set -l time (string sub -s 2 -- \"\$timr_status\")
            if test \"\$icon\" = \"▶\"
                set_color green
            else
                set_color yellow
            end
            printf '%s' \"\$icon\"
            set_color normal
            printf ' %s' \"\$time\"
        end
    end
end

complete -c timr -n __fish_use_subcommand -a start -d \"Start the timer\"
complete -c timr -n __fish_use_subcommand -a stop -d \"Stop the timer\"
complete -c timr -n __fish_use_subcommand -a pause -d \"Pause the timer\"
complete -c timr -n __fish_use_subcommand -a status -d \"Show the timer status\"
complete -c timr -n __fish_use_subcommand -a reset -d \"Reset the timer\"
complete -c timr -n __fish_use_subcommand -a live -d \"Show the live timer\"
complete -c timr -n __fish_use_subcommand -a prompt -d \"Show the prompt status\"
" > "$integration_file"

echo "Created: $integration_file"
echo ""
echo "Installation complete!"
echo ""
echo "To display timr in your prompt, add this to your fish_prompt or fish_right_prompt:"
echo ""
echo "  function fish_prompt"
echo "      # ... your existing prompt ..."
echo "      printf '%s ' (timr_prompt)"
echo "  end"
echo ""
echo "Restart your fish shell or run 'source $integration_file' to apply changes."
