# Builds lcsclient.exe as a GUI-subsystem binary (no console window).
# Plain `go build` defaults to the console subsystem, which pops up a
# black console window alongside the app's own window.
#
# Bump $version on each release build -- it's stamped into the binary
# and shown in Help > About.
$version = "0.0.3d"

go build -ldflags="-H=windowsgui -X lcsclient/internal/version.Version=$version" -o bin\lcsclient.exe .\cmd\lcsclient
