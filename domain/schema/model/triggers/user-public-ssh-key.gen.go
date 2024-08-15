// Code generated by triggergen. DO NOT EDIT.

package triggers

import (
	"fmt"
	"strings"

	"github.com/juju/juju/core/database/schema"
)


// ChangeLogTriggersForUserPublicSshKey generates the triggers for the
// user_public_ssh_key table.
func ChangeLogTriggersForUserPublicSshKey(namespaceID int, changeColumnName string) func() schema.Patch {
	return ChangeLogTriggersForUserPublicSshKeyWithDiscriminator(namespaceID, changeColumnName, "")
}

// ChangeLogTriggersForUserPublicSshKeyWithDiscriminator generates the triggers for the
// user_public_ssh_key table, with the value of the optional discriminator column included in the
// change event. The discriminator column name is ignored if empty.
func ChangeLogTriggersForUserPublicSshKeyWithDiscriminator(namespaceID int, changeColumnName, discriminatorColumnName string) func() schema.Patch {
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
-- insert trigger for UserPublicSshKey
CREATE TRIGGER trg_log_user_public_ssh_key_insert
AFTER INSERT ON user_public_ssh_key FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, %[4]s, created_at)
    VALUES (1, %[1]d, %[2]s, DATETIME('now'));
END;

-- update trigger for UserPublicSshKey
CREATE TRIGGER trg_log_user_public_ssh_key_update
AFTER UPDATE ON user_public_ssh_key FOR EACH ROW
WHEN 
	NEW.comment != OLD.comment OR
	NEW.fingerprint_hash_algorithm_id != OLD.fingerprint_hash_algorithm_id OR
	NEW.fingerprint != OLD.fingerprint OR
	NEW.public_key != OLD.public_key OR
	NEW.user_id != OLD.user_id 
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, %[4]s, created_at)
    VALUES (2, %[1]d, %[3]s, DATETIME('now'));
END;

-- delete trigger for UserPublicSshKey
CREATE TRIGGER trg_log_user_public_ssh_key_delete
AFTER DELETE ON user_public_ssh_key FOR EACH ROW
BEGIN
    INSERT INTO change_log (edit_type_id, namespace_id, %[4]s, created_at)
    VALUES (4, %[1]d, %[3]s, DATETIME('now'));
END;`, namespaceID, newColumnValues, oldColumnValues, strings.Join(changeLogColumns, ", ")))
	}
}

