-- M6: удаление аккаунта и доступ к исследовательским данным.
-- §18.5: удаление аккаунта удаляет получателей, атрибуты и research-связку (каскады уже в Прил. A);
-- ссылки «кто сделал» (audit, замены, разметка, версии нумерологии, экспорты) не должны мешать удалению —
-- они обнуляются (ON DELETE SET NULL), а сами записи остаются для журнала.
-- RM-03: связка research_id ↔ user_id доступна только роли research_admin (и владельцу схемы — приложению).

-- +goose Up
ALTER TABLE audit_events DROP CONSTRAINT audit_events_actor_id_fkey,
    ADD CONSTRAINT audit_events_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE substitutions DROP CONSTRAINT substitutions_proposed_by_fkey,
    ADD CONSTRAINT substitutions_proposed_by_fkey FOREIGN KEY (proposed_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE expert_judgements DROP CONSTRAINT expert_judgements_expert_id_fkey,
    ADD CONSTRAINT expert_judgements_expert_id_fkey FOREIGN KEY (expert_id) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE numerology_versions DROP CONSTRAINT numerology_versions_created_by_fkey,
    ADD CONSTRAINT numerology_versions_created_by_fkey FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE research.research_exports DROP CONSTRAINT research_exports_requested_by_fkey,
    ADD CONSTRAINT research_exports_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES users(id) ON DELETE SET NULL;

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'research_admin') THEN
        CREATE ROLE research_admin NOLOGIN;
    END IF;
END $$;
-- +goose StatementEnd
REVOKE ALL ON ALL TABLES IN SCHEMA research FROM PUBLIC;
GRANT USAGE ON SCHEMA research TO research_admin;
GRANT SELECT ON research.research_subjects TO research_admin;

-- +goose Down
REVOKE SELECT ON research.research_subjects FROM research_admin;
REVOKE USAGE ON SCHEMA research FROM research_admin;
ALTER TABLE research.research_exports DROP CONSTRAINT research_exports_requested_by_fkey,
    ADD CONSTRAINT research_exports_requested_by_fkey FOREIGN KEY (requested_by) REFERENCES users(id);
ALTER TABLE numerology_versions DROP CONSTRAINT numerology_versions_created_by_fkey,
    ADD CONSTRAINT numerology_versions_created_by_fkey FOREIGN KEY (created_by) REFERENCES users(id);
ALTER TABLE expert_judgements DROP CONSTRAINT expert_judgements_expert_id_fkey,
    ADD CONSTRAINT expert_judgements_expert_id_fkey FOREIGN KEY (expert_id) REFERENCES users(id);
ALTER TABLE substitutions DROP CONSTRAINT substitutions_proposed_by_fkey,
    ADD CONSTRAINT substitutions_proposed_by_fkey FOREIGN KEY (proposed_by) REFERENCES users(id);
ALTER TABLE audit_events DROP CONSTRAINT audit_events_actor_id_fkey,
    ADD CONSTRAINT audit_events_actor_id_fkey FOREIGN KEY (actor_id) REFERENCES users(id);
