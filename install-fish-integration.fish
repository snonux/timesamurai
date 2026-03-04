#!/usr/bin/env fish

set -l fish_config_dir (set -q XDG_CONFIG_HOME; and echo "$XDG_CONFIG_HOME/fish"; or echo "$HOME/.config/fish")
set -l fish_conf_d "$fish_config_dir/conf.d"
set -l integration_file "$fish_conf_d/timesamurai.fish"

echo "Installing timesamurai fish shell integration..."

# Create fish config directory if it doesn't exist
if not test -d "$fish_conf_d"
    mkdir -p "$fish_conf_d"
    echo "Created directory: $fish_conf_d"
end

# Write the integration file
echo "\
function timesamurai_prompt -d \"Display timesamurai timesamurai_status in the prompt\"
    if command -v timesamurai >/dev/null
        set -l timesamurai_status (timesamurai prompt)
        if test -n \"\$timesamurai_status\"
            set -l icon (string sub -l 1 -- \"\$timesamurai_status\")
            set -l time (string sub -s 2 -- \"\$timesamurai_status\")
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

complete -c timesamurai -n __fish_use_subcommand -a start -d \"Start the timer\"
complete -c timesamurai -n __fish_use_subcommand -a stop -d \"Stop the timer\"
complete -c timesamurai -n __fish_use_subcommand -a pause -d \"Pause the timer\"
complete -c timesamurai -n __fish_use_subcommand -a continue -d \"Continue the timer\"
complete -c timesamurai -n __fish_use_subcommand -a cont -d \"Continue the timer\"
complete -c timesamurai -n __fish_use_subcommand -a status -d \"Show the timer status\"
complete -c timesamurai -n __fish_use_subcommand -a reset -d \"Reset the timer\"
complete -c timesamurai -n __fish_use_subcommand -a track -d \"Save time with description\"
complete -c timesamurai -n __fish_use_subcommand -a live -d \"Show the live timer\"
complete -c timesamurai -n __fish_use_subcommand -a prompt -d \"Show the prompt status\"

# Font completions for live mode
complete -c timesamurai -n \"__fish_seen_subcommand_from live\" -s f -l font -d \"Font style\" -a \"doom mono12 rebel ansi ansiShadow\"
" > "$integration_file"

echo "Created: $integration_file"
echo ""
echo "Installation complete!"
echo ""
echo "To display timesamurai in your prompt, add this to your fish_prompt or fish_right_prompt:"
echo ""
echo "  function fish_prompt"
echo "      # ... your existing prompt ..."
echo "      printf '%s ' (timesamurai_prompt)"
echo "  end"
echo ""
echo "Restart your fish shell or run 'source $integration_file' to apply changes."
