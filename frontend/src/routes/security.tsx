import { createFileRoute } from '@tanstack/react-router'
import { KeyRound, Lock, ShieldCheck } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'

import { PageHeader } from '@/components/page-header'

const POINTS: { icon: LucideIcon; title: string; body: string }[] = [
  {
    icon: KeyRound,
    title: 'No passwords, ever',
    body: 'You sign in on Spotify, Apple Music, or YouTube Music through their own OAuth screens. Aurify only ever receives a scoped access token, so your streaming password never reaches our servers.',
  },
  {
    icon: ShieldCheck,
    title: 'Least privilege',
    body: 'Aurify asks only to read the playlists you connect. It never requests permission to post, follow, or change anything on your account, and the token it holds is short lived.',
  },
  {
    icon: Lock,
    title: 'Nothing sensitive is kept',
    body: 'Covers are built from aggregated audio features and overall lyric sentiment. Your raw lyrics and listening history are analyzed in the moment, not stored.',
  },
]

export const Route = createFileRoute('/security')({
  component: SecurityPage,
})

function SecurityPage() {
  return (
    <div className="max-w-2xl space-y-8">
      <PageHeader
        title="Security is a focus here, not a vibe"
        description="Spotify, Apple Music, and YouTube Music sign-ins happen through their own platforms. Aurify receives a safe, ephemeral, minimally scoped access token, never your password."
      />

      <ul className="space-y-6">
        {POINTS.map(({ icon: Icon, title, body }) => (
          <li key={title} className="flex gap-3">
            <span
              aria-hidden="true"
              className="mt-0.5 flex size-9 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground"
            >
              <Icon className="size-5" />
            </span>
            <div className="space-y-1">
              <h2 className="font-display font-semibold tracking-tight">{title}</h2>
              <p className="text-pretty text-sm text-muted-foreground">{body}</p>
            </div>
          </li>
        ))}
      </ul>
    </div>
  )
}
