import { api } from '../../shared/api/client';

export type UserRole = 'ADMIN' | 'PLATFORM_ENGINEER' | 'DEVELOPER';
export type UserAccount = { id: string; username: string; role: UserRole; status: 'ACTIVE' | 'DISABLED' };

export function listUsers(): Promise<{ users: UserAccount[] }> {
  return api<{ users: UserAccount[] }>('/users');
}

export function createUser(username: string, password: string, role: UserRole): Promise<{ user: UserAccount }> {
  return api<{ user: UserAccount }>('/users', { method: 'POST', body: JSON.stringify({ username, password, role }) });
}

// Admin password reset (UC-00 BR-11): the backend revokes every session of
// the target account, so it signs in again with the new password.
export function resetUserPassword(id: string, newPassword: string, confirmPassword: string): Promise<void> {
  return api<void>(`/users/${encodeURIComponent(id)}/reset-password`, { method: 'POST', body: JSON.stringify({ newPassword, confirmPassword }) });
}
