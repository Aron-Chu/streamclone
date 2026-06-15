import { Link } from 'react-router-dom'

interface ShowAllLinkProps {
  to: string
  children?: string
}

export function ShowAllLink({ to, children = 'Show all' }: ShowAllLinkProps) {
  return (
    <Link
      to={to}
      className="text-sm font-bold text-[#9147ff] transition hover:text-[#bf94ff] hover:underline"
    >
      {children}
    </Link>
  )
}
