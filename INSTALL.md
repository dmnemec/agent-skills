# Install vault-curator (instructions for an agent)

The user asked you to install vault-curator from this repo. Do the steps below yourself; the user only answers your questions.

1. Clone `https://github.com/dmnemec/agent-skills` to `~/agent-skills`, or run `git -C ~/agent-skills pull` if it exists. Done when `~/agent-skills/skills/vault-curator/setup/SKILL.md` exists.
2. Find where your own product reads skills and global instructions: the section of `adapters/README.md` in the clone for your product (`#claude-code`, `#openai-codex`, `#github-copilot`, `#code-puppy`). Where the section says "Unverified", try the assumed location, tell the user, and ask them if it does not work. Note the global instructions file it names; setup step 3 makes that file load the vault.
3. Announce it, then link `~/agent-skills/skills/vault-curator/setup` there as `vault-curator-setup`: a symlink, or on Windows a directory junction (`mklink /J <skills location>\vault-curator-setup %USERPROFILE%\agent-skills\skills\vault-curator\setup`). Copy the folder only if linking fails; a copy cannot reach the repo's `docs/`. Done when `<skills location>/vault-curator-setup/SKILL.md` exists.
4. Read that `SKILL.md` and follow it from step 1. If your product only loads skills at session start, follow it directly from the file now.

Re-running this file on a machine that is already set up changes nothing.
