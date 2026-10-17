# Security policy

Please report vulnerabilities in the LayerHop Go SDK privately to **security@layerhop.com**. Do not open a public
issue, discussion or pull request for a security problem.

Include the affected package and version, steps to reproduce, and the impact you observed. We
acknowledge reports within 3 business days and keep you updated until a fix is released.

LayerHop is non-custodial: the SDKs never hold private keys and never sign transactions. They
fetch quotes and build *unsigned* transactions that the user's own wallet signs.

## Supported versions

Only the latest released version receives security fixes.
