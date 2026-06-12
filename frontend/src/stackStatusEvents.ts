export const OPEN_STACK_STATUS_EVENT = 'streamclone:open-stack-status'

export function dispatchOpenStackStatus() {
  window.dispatchEvent(new CustomEvent(OPEN_STACK_STATUS_EVENT))
}
