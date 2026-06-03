import { useMemo, useState } from "react";
import { Button } from "@/components/ui";
import { displayTaskRoom, displayTaskTeam, formatTaskUpdatedAt, groupTasksByParent } from "@/models/tasks";
import "./TasksView.css";

export function TasksView({
  t,
  tasks,
  selectedTask,
  selectedTaskID,
  loading,
  error,
  onRefresh,
  onSelectTask,
  onOpenConversation,
}) {
  const groups = useMemo(() => groupTasksByParent(tasks), [tasks]);
  const [expandedParents, setExpandedParents] = useState<Record<string, boolean>>({});

  function isParentExpanded(parentID: string, hasSelectedChild: boolean) {
    return Boolean(expandedParents[parentID]) || hasSelectedChild;
  }

  function toggleParent(parentID: string) {
    setExpandedParents((current) => ({
      ...current,
      [parentID]: !current[parentID],
    }));
  }

  return (
    <section className="entity-pane tasks-pane">
      <header className="tasks-page-header">
        <div className="tasks-page-heading">
          <h1>{t("tasksPageTitle")}</h1>
          <p>{t("tasksPageSubtitle")}</p>
        </div>
        <Button variant="secondaryGray" size="md" onClick={onRefresh}>
          {t("tasksRefresh")}
        </Button>
      </header>
      {error ? <div className="form-error">{error}</div> : null}
      {!loading && !error && tasks.length === 0 ? (
        <div className="empty-state shell-empty-state">
          <strong>{t("tasksEmpty")}</strong>
          <span>{t("tasksEmptyHint")}</span>
        </div>
      ) : null}
      {loading && !tasks.length ? <div className="workspace-empty">{t("tasksLoading")}</div> : null}
      {!loading && !error && tasks.length ? (
        <div className="tasks-workbench">
          <div className="tasks-list-panel">
            <div className="tasks-list-head">
              <span>{t("tasksListLabel")}</span>
              <strong>{groups.length}</strong>
            </div>
            <div className="tasks-list">
              {groups.map((group) => {
                const hasSelectedChild = group.children.some((child) => child.id === selectedTaskID);
                const expanded = isParentExpanded(group.task.id, hasSelectedChild);
                return (
                  <div key={group.task.id} className="task-group">
                    <div className={`task-row task-row-parent ${selectedTaskID === group.task.id ? "active" : ""}`}>
                      <div className="task-row-top">
                        <span className={`status-pill task-status-pill task-status-${group.task.status}`}>
                          {group.task.status}
                        </span>
                        <span className="task-updated-at">
                          {formatTaskUpdatedAt(group.task.updated_at, document.documentElement.lang)}
                        </span>
                      </div>
                      <div className="task-row-title-line">
                        <button type="button" className="task-row-select" onClick={() => onSelectTask(group.task.id)}>
                          <h2>{group.task.title}</h2>
                        </button>
                        {group.children.length ? (
                          <button
                            type="button"
                            className="task-group-toggle"
                            onClick={(event) => {
                              event.stopPropagation();
                              toggleParent(group.task.id);
                            }}
                          >
                            {expanded ? t("taskHideChildren") : t("taskShowChildren", { count: group.children.length })}
                          </button>
                        ) : null}
                      </div>
                      <div className="task-meta-grid">
                        <span>
                          {t("taskKindLabel")} {t("taskKindParent")}
                        </span>
                        <span>
                          {t("taskPriorityLabel")} {group.task.priority || 0}
                        </span>
                        <span>
                          {t("taskTeamLabel")} {displayTaskTeam(group.task)}
                        </span>
                        <span>
                          {t("taskRoomLabel")} {displayTaskRoom(group.task)}
                        </span>
                      </div>
                    </div>
                    {expanded && group.children.length ? (
                      <div className="task-children-list">
                        {group.children.map((child) => (
                          <button
                            key={child.id}
                            type="button"
                            className={`task-row task-row-child ${selectedTaskID === child.id ? "active" : ""}`}
                            onClick={() => onSelectTask(child.id)}
                          >
                            <div className="task-row-top">
                              <span className={`status-pill task-status-pill task-status-${child.status}`}>
                                {child.status}
                              </span>
                              <span className="task-updated-at">
                                {formatTaskUpdatedAt(child.updated_at, document.documentElement.lang)}
                              </span>
                            </div>
                            <h3>{child.title}</h3>
                            <div className="task-meta-grid">
                              <span>
                                {t("taskKindLabel")} {t("taskKindChild")}
                              </span>
                              <span>
                                {t("taskAssigneeLabel")} {child.assigned_to || "-"}
                              </span>
                              <span>
                                {t("taskTeamLabel")} {displayTaskTeam(child)}
                              </span>
                              <span>
                                {t("taskRoomLabel")} {displayTaskRoom(child)}
                              </span>
                            </div>
                          </button>
                        ))}
                      </div>
                    ) : null}
                  </div>
                );
              })}
            </div>
          </div>
          <div className="tasks-detail-panel">
            {selectedTask ? (
              <>
                <div className="tasks-detail-header">
                  <div>
                    <div className="tasks-detail-kicker">{t("tasksDetailLabel")}</div>
                    <h2>{selectedTask.title}</h2>
                  </div>
                  <Button variant="primary" size="md" onClick={() => onOpenConversation(selectedTask.room_id)}>
                    {t("taskOpenConversation")}
                  </Button>
                </div>
                <div className="tasks-detail-grid">
                  <div className="entity-field">
                    <span>{t("taskKindLabel")}</span>
                    <strong>{selectedTask.parent_id ? t("taskKindChild") : t("taskKindParent")}</strong>
                  </div>
                  <div className="entity-field">
                    <span>{t("taskStatusLabel")}</span>
                    <strong>{selectedTask.status}</strong>
                  </div>
                  <div className="entity-field">
                    <span>{t("taskAssigneeLabel")}</span>
                    <strong>{selectedTask.assigned_to || "-"}</strong>
                  </div>
                  <div className="entity-field">
                    <span>{t("taskParentLabel")}</span>
                    <strong>{selectedTask.parent_id || "-"}</strong>
                  </div>
                  <div className="entity-field">
                    <span>{t("taskTeamLabel")}</span>
                    <strong>{displayTaskTeam(selectedTask)}</strong>
                  </div>
                  <div className="entity-field">
                    <span>{t("taskRoomLabel")}</span>
                    <strong>{displayTaskRoom(selectedTask)}</strong>
                  </div>
                  <div className="entity-field">
                    <span>{t("taskPriorityLabel")}</span>
                    <strong>{selectedTask.priority || 0}</strong>
                  </div>
                  <div className="entity-field">
                    <span>{t("taskUpdatedAtLabel")}</span>
                    <strong>{formatTaskUpdatedAt(selectedTask.updated_at, document.documentElement.lang)}</strong>
                  </div>
                </div>
                <div className="tasks-detail-section">
                  <h3>{t("taskDescriptionLabel")}</h3>
                  <p>{selectedTask.body || t("tasksDetailPlaceholder")}</p>
                </div>
                <div className="tasks-detail-section">
                  <h3>{t("taskDependsOnLabel")}</h3>
                  <p>{selectedTask.depends_on.length ? selectedTask.depends_on.join(", ") : "-"}</p>
                </div>
              </>
            ) : (
              <div className="workspace-empty">{t("tasksSelectHint")}</div>
            )}
          </div>
        </div>
      ) : null}
    </section>
  );
}
