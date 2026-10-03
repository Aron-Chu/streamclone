export function validateAuditSchema(audit: unknown): string[]
export function validateAuditPolicy(audit: unknown, lock: unknown, npmStatus: number | null, production?: boolean): string[]
export function assertBuildOnlyRuntimeBoundary(bundle: Record<string, unknown>): void
