# Ops migration manifest (historical)

This manifest recorded the **streamclone → streampulse-ops** file move during the 2026 boundary split. Active production deploy, secrets, SSH runbooks, and promotion evidence live in **private streampulse-ops** only.

Public Streamclone keeps core compose, desktop install, and local dev docs. Do not reintroduce hosted production paths, BearHost profile names, or operator topology in this tree.

See [`streampulse-product-boundary.md`](streampulse-product-boundary.md) for the current product split.
