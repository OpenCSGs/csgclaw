package team

type GlobalTaskDirectory interface {
	RoomTitle(roomID string) (string, bool)
}

type GlobalTaskView struct {
	Task      TeamTask
	TeamTitle string
	RoomTitle string
}

func (s *Service) ListGlobalTaskViews(directory GlobalTaskDirectory) []GlobalTaskView {
	tasks := s.ListAllTasks()
	out := make([]GlobalTaskView, 0, len(tasks))
	for _, task := range tasks {
		view := GlobalTaskView{Task: task}
		if meta, found := s.GetTeam(task.TeamID); found {
			view.TeamTitle = meta.Title
		}
		if directory != nil {
			if title, found := directory.RoomTitle(task.RoomID); found {
				view.RoomTitle = title
			}
		}
		out = append(out, view)
	}
	return out
}
