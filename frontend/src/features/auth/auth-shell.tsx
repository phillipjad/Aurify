import type { ReactNode } from 'react'
import { AlertCircle, CheckCircle2 } from 'lucide-react'

import { Card, CardContent } from '@/components/ui/card'

/**
 * Read a text field out of a form.
 *
 * FormData.get returns `string | File | null`, and a File would stringify to
 * "[object File]" — a value that would be sent as a password or an address
 * rather than rejected. Narrowing to string keeps that out of the request body.
 */
export function textField(form: FormData, name: string): string {
  const value = form.get(name)
  return typeof value === 'string' ? value : ''
}

/** Shared field styling for every auth input, so the forms cannot drift apart. */
export const fieldClass =
  'h-9 w-full rounded-lg border border-border bg-card px-3 text-sm text-foreground placeholder:text-muted-foreground disabled:opacity-50 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background'

interface AuthShellProps {
  title: string
  description?: ReactNode
  children: ReactNode
  /** Rendered under the card — sign-in/sign-up cross-links, reset links. */
  footer?: ReactNode
}

/** The centered card every auth screen sits in. */
export function AuthShell({ title, description, children, footer }: AuthShellProps) {
  return (
    <div className="mx-auto max-w-sm space-y-6">
      <div className="space-y-1.5 text-center">
        <h1 className="font-display text-2xl font-bold tracking-tight">{title}</h1>
        {description ? <div className="text-pretty text-sm text-muted-foreground">{description}</div> : null}
      </div>

      <Card>
        <CardContent className="pt-5">{children}</CardContent>
      </Card>

      {footer ? <div className="text-center text-sm text-muted-foreground">{footer}</div> : null}
    </div>
  )
}

/**
 * An error from the API, shown verbatim.
 *
 * The wording is deliberately the server's: it is the side that knows whether a
 * message may disclose account existence, how many attempts remain before a
 * lockout, and where to go for help. Paraphrasing here would lose all of that.
 */
export function FormError({ message }: { message?: string }) {
  if (!message) return null
  return (
    <p
      role="alert"
      className="flex items-start gap-2 rounded-lg border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive"
    >
      <AlertCircle aria-hidden="true" className="mt-0.5 size-4 shrink-0" />
      <span className="text-pretty">{message}</span>
    </p>
  )
}

/** A success acknowledgement, styled as the counterpart of FormError. */
export function FormSuccess({ message }: { message?: string }) {
  if (!message) return null
  return (
    <p
      role="status"
      className="flex items-start gap-2 rounded-lg border border-primary/40 bg-primary/10 px-3 py-2 text-sm text-foreground"
    >
      <CheckCircle2 aria-hidden="true" className="mt-0.5 size-4 shrink-0 text-primary" />
      <span className="text-pretty">{message}</span>
    </p>
  )
}

interface FieldProps {
  id: string
  label: string
  type?: string
  autoComplete?: string
  required?: boolean
  hint?: ReactNode
  defaultValue?: string
}

/** A labelled input. The label is a real <label>, never a placeholder. */
export function Field({ id, label, type = 'text', autoComplete, required, hint, defaultValue }: FieldProps) {
  const hintId = hint ? `${id}-hint` : undefined
  return (
    <div className="space-y-1.5">
      <label htmlFor={id} className="text-sm font-medium">
        {label}
      </label>
      <input
        id={id}
        name={id}
        type={type}
        autoComplete={autoComplete}
        required={required}
        defaultValue={defaultValue}
        aria-describedby={hintId}
        className={fieldClass}
      />
      {hint ? (
        <p id={hintId} className="text-xs text-muted-foreground">
          {hint}
        </p>
      ) : null}
    </div>
  )
}
