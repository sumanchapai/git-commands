# Git Commands

This is a go server that allows users to run specific git commands on a
specificied directory via HTML frontend.

Run as follows:

```
GIT_REPO_PATH="$HOME/Desktop/projects/superview-accounting" go run main.go
```

setting the GIT_REPO_PATH as desired.

Running in launchctl as daemon as:

```
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.user.git-commands</string>

    <key>ProgramArguments</key>
    <array>
        <string>/opt/homebrew/bin/go</string>
        <string>run</string>
        <string>/Users/suman/Desktop/projects/git-commands/main.go</string>
    </array>

    <key>EnvironmentVariables</key>
    <dict>
        <key>GIT_REPO_PATH</key>
        <string>/Users/suman/Desktop/projects/superview-accounting</string>
    </dict>

    <key>KeepAlive</key>
    <true/>

    <key>RunAtLoad</key>
    <true/>

    <key>StandardOutPath</key>
    <string>/tmp/git-commands.log</string>

    <key>StandardErrorPath</key>
    <string>/tmp/git-commands.err</string>
</dict>
</plist>

```
