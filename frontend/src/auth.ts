import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { claimPreparedDevTwitchToken, getMe, importDevTwitchToken, logout } from './api'

export function useAuth() {
  const queryClient = useQueryClient()
  const me = useQuery({
    queryKey: ['me'],
    queryFn: getMe,
    staleTime: 30_000,
  })
  const logoutMutation = useMutation({
    mutationFn: logout,
    onSuccess: () => {
      queryClient.setQueryData(['me'], (current: ReturnType<typeof getMe> extends Promise<infer T> ? T | undefined : never) => ({
        ...current,
        authenticated: false,
        user: undefined,
        scopes: undefined,
      }))
      queryClient.invalidateQueries({ queryKey: ['me'] })
      queryClient.invalidateQueries({ queryKey: ['followed'] })
      queryClient.invalidateQueries({ queryKey: ['followed', 'local'] })
    },
  })
  const importMutation = useMutation({
    mutationFn: importDevTwitchToken,
    onSuccess: data => {
      queryClient.setQueryData(['me'], (current: ReturnType<typeof getMe> extends Promise<infer T> ? T | undefined : never) => ({
        ...current,
        authenticated: data.authenticated,
        user: data.user,
        scopes: data.scopes,
      }))
      queryClient.invalidateQueries({ queryKey: ['me'] })
      queryClient.invalidateQueries({ queryKey: ['followed'] })
      queryClient.invalidateQueries({ queryKey: ['followed', 'local'] })
    },
  })
  const claimPreparedMutation = useMutation({
    mutationFn: claimPreparedDevTwitchToken,
    onSuccess: data => {
      queryClient.setQueryData(['me'], (current: ReturnType<typeof getMe> extends Promise<infer T> ? T | undefined : never) => ({
        ...current,
        authenticated: data.authenticated,
        user: data.user,
        scopes: data.scopes,
      }))
      queryClient.invalidateQueries({ queryKey: ['me'] })
      queryClient.invalidateQueries({ queryKey: ['followed'] })
      queryClient.invalidateQueries({ queryKey: ['followed', 'local'] })
    },
  })

  return {
    me: me.data,
    isLoading: me.isLoading,
    error: me.error,
    isAuthenticated: Boolean(me.data?.authenticated),
    user: me.data?.user,
    canImportLocalToken: Boolean(me.data?.canImportLocalToken),
    importLocalToken: importMutation.mutateAsync,
    isImportingLocalToken: importMutation.isPending,
    claimPreparedLocalToken: claimPreparedMutation.mutateAsync,
    isClaimingPreparedLocalToken: claimPreparedMutation.isPending,
    logout: () => logoutMutation.mutate(),
  }
}
