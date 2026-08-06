export class DeferredRelaunch {
  private requested = false;
  private scheduled = false;

  request(): boolean {
    if (this.requested) {
      return false;
    }
    this.requested = true;
    return true;
  }

  scheduleIfRequested(schedule: () => void): boolean {
    if (!this.requested || this.scheduled) {
      return false;
    }
    this.scheduled = true;
    schedule();
    return true;
  }
}
