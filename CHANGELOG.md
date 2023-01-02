# CHANGELOG

# v0.3.0

* Update to Go 1.18 generics. Now nodes operation is faster and type safe.
* Breaking changes: some types changed the name, but most code should keep working if it
  uses variable inference instead of explicit type declarations.

# v0.2.0

* Inter-node communication input channels are now unbuffered by default. To make them buffered,
  you can append the `node.ChannelBufferLen` function to the `AsMiddle` and `AsTerminal` functions.

# v0.1.1

* Added InType and OutType inspection functions to the Nodes

# v0.1.0

* Initial import from github.com/mariomac/go-pipes
