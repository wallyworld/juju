// Code generated by triggergen. DO NOT EDIT.

package triggers

import (
	"fmt"
	"strings"

	"github.com/juju/juju/core/database/schema"
)


// ChangeLogTriggersForApplicationScale generates the triggers for the
// application_scale table.
func ChangeLogTriggersForApplicationScale(namespaceID int, changeColumnName string) func() schema.Patch {
	return ChangeLogTriggersForApplicationScaleWithDiscriminator(namespaceID, changeColumnName, "")
}

// ChangeLogTriggersForApplicationScaleWithDiscriminator generates the triggers for the
// application_scale table, with the value of the optional discriminator column included in the
// change event. The discriminator column name is ignored if empty.
func ChangeLogTriggersForApplicationScaleWithDiscriminator(namespaceID int, changeColumnName, discriminatorColumnName string) func() schema.Patch {
	changeLogColumns := []string{"changed"}
	newColumnValues := "NEW." + changeColumnName
	oldColumnValues := "OLD." + changeColumnName
	if discriminatorColumnName != "" {
		changeLogColumns = append(changeLogColumns, "discriminator")
		newColumnValues += ", NEW." + discriminatorColumnName
		oldColumnValues += ", OLD." + discriminatorColumnName
	}
	return func() schema.Patch {
		return schema.MakePatch(fmt.Sprintf(`
-- insert trigger for ApplicationScale
CREATE TRIGGER trg_log_application_scale_insert
AFTER INSERT ON application_scale FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, %[4]s, created_at)
    VALUES (1, %[1]d, %[2]s, DATETIME('now'));
END;

-- update trigger for ApplicationScale
CREATE TRIGGER trg_log_application_scale_update
AFTER UPDATE ON application_scale FOR EACH ROW
WHEN 
	(NEW.scale != OLD.scale OR (NEW.scale IS NOT NULL AND OLD.scale IS NULL) OR (NEW.scale IS NULL AND OLD.scale IS NOT NULL)) OR
	(NEW.scale_target != OLD.scale_target OR (NEW.scale_target IS NOT NULL AND OLD.scale_target IS NULL) OR (NEW.scale_target IS NULL AND OLD.scale_target IS NOT NULL)) OR
	(NEW.scaling != OLD.scaling OR (NEW.scaling IS NOT NULL AND OLD.scaling IS NULL) OR (NEW.scaling IS NULL AND OLD.scaling IS NOT NULL)) 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, %[4]s, created_at)
    VALUES (2, %[1]d, %[3]s, DATETIME('now'));
END;

-- delete trigger for ApplicationScale
CREATE TRIGGER trg_log_application_scale_delete
AFTER DELETE ON application_scale FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, %[4]s, created_at)
    VALUES (4, %[1]d, %[3]s, DATETIME('now'));
END;`, namespaceID, newColumnValues, oldColumnValues, strings.Join(changeLogColumns, ", ")))
	}
}

