-- Drop schema no Go code reads or writes.
--
-- casbin_rule survived the move from Casbin to OPA. There is no Casbin import
-- in go.mod and no reference to the table anywhere in the tree.
--
-- labels was created in anticipation of a labels feature that was never built:
-- there is no labels storage package, no query touches it, and
-- resource.ResourceTypeLabel has no call site either. Its role matrix rows in
-- authz.rego are equally unreachable.
--
-- A table nobody reads still costs something: it appears in schema diffs, it
-- gets migrated and rebuilt, and it reads as a feature to anyone new.

DROP TABLE IF EXISTS labels;
DROP TABLE IF EXISTS casbin_rule;
