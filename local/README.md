To work with a local dependent module, check out that module into this
directory, and put the local replace into the main go.mod.

Example:

```bash
cd local
git clone  git@github.com:Bolodya1997/sdk.git
cd sdk
git remote add upstream git@github.com:networkservicemesh/sdk.git
cd ../..
```

And then edit the go.mod file utilize a replace:

```vgo
module github.com/networkservicemesh/cmd-forwarder-sriov

go 1.15

require (
...
)

replace github.com/networkservicemesh/sdk => ./local/sdk
```

