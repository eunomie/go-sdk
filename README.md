# Go SDK

Develop Dagger modules and clients in Go.

## Manifest pins

This SDK uses `sdk-helpers@v1` to generate module manifests.
The `lock` SDK setting defaults to `false`. A pin is a manifest field that stores
a selected Git commit. Default output has no pin fields. Set `lock` to `true`
to write selected commits as pins. Local dependencies need no pin.

Source references stay intact, including any commit in a source reference.
Commit data used by generated clients also stays intact.
