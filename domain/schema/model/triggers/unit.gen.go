// Code generated by triggergen. DO NOT EDIT.

package triggers

import (
	"fmt"

	"github.com/juju/juju/core/database/schema"
)


// ChangeLogTriggersForUnit generates the triggers for the
// unit table.
func ChangeLogTriggersForUnit(columnName string, namespaceID int) func() schema.Patch {
	return func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
-- insert namespace for Unit
INSERT INTO change_log_namespace VALUES (%[2]d, 'unit', 'Unit changes based on %[1]s');

-- insert trigger for Unit
CREATE TRIGGER trg_log_unit_insert
AFTER INSERT ON unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (1, %[2]d, NEW.%[1]s, DATETIME('now'));
END;

-- update trigger for Unit
CREATE TRIGGER trg_log_unit_update
AFTER UPDATE ON unit FOR EACH ROW
WHEN 
	NEW.name != OLD.name OR
	NEW.life_id != OLD.life_id OR
	NEW.application_uuid != OLD.application_uuid OR
	NEW.net_node_uuid != OLD.net_node_uuid OR
	(NEW.charm_uuid != OLD.charm_uuid OR (NEW.charm_uuid IS NOT NULL AND OLD.charm_uuid IS NULL) OR (NEW.charm_uuid IS NULL AND OLD.charm_uuid IS NOT NULL)) OR
	NEW.resolve_kind_id != OLD.resolve_kind_id OR
	(NEW.password_hash_algorithm_id != OLD.password_hash_algorithm_id OR (NEW.password_hash_algorithm_id IS NOT NULL AND OLD.password_hash_algorithm_id IS NULL) OR (NEW.password_hash_algorithm_id IS NULL AND OLD.password_hash_algorithm_id IS NOT NULL)) OR
	(NEW.password_hash != OLD.password_hash OR (NEW.password_hash IS NOT NULL AND OLD.password_hash IS NULL) OR (NEW.password_hash IS NULL AND OLD.password_hash IS NOT NULL)) 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (2, %[2]d, OLD.%[1]s, DATETIME('now'));
END;

-- delete trigger for Unit
CREATE TRIGGER trg_log_unit_delete
AFTER DELETE ON unit FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, changed, created_at)
    VALUES (4, %[2]d, OLD.%[1]s, DATETIME('now'));
END;`, columnName, namespaceID))
	}
}

