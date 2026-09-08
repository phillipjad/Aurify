import { describe, expect, it, vi } from 'vite-plus/test'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

import { Button } from '@/components/ui/button'

describe('Button', () => {
  it('renders its children', () => {
    render(<Button>Click me</Button>)
    expect(screen.getByRole('button', { name: 'Click me' })).toBeInTheDocument()
  })

  it('calls onClick when pressed', async () => {
    const onClick = vi.fn()
    render(<Button onClick={onClick}>Go</Button>)

    await userEvent.click(screen.getByRole('button', { name: 'Go' }))

    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('does not fire onClick when disabled', async () => {
    const onClick = vi.fn()
    render(
      <Button disabled onClick={onClick}>
        Nope
      </Button>,
    )

    await userEvent.click(screen.getByRole('button', { name: 'Nope' }))

    expect(onClick).not.toHaveBeenCalled()
  })

  it('applies the requested variant and size classes', () => {
    render(
      <Button variant="outline" size="icon">
        Icon
      </Button>,
    )
    const button = screen.getByRole('button', { name: 'Icon' })
    expect(button).toHaveClass('border') // outline variant
    expect(button).toHaveClass('h-10', 'w-10') // icon size
  })

  // The busy marker is what exempts the spinner from the global reduced-motion
  // reset in styles.css. Without it the reset froze the spinner on its first
  // frame, so a working button was indistinguishable from a dead one for anyone
  // with the OS preference on. jsdom applies no stylesheets, so this pins the
  // hook the CSS keys off rather than the animation itself.
  it('marks the loading spinner as a busy indicator', () => {
    const { container } = render(<Button loading>Saving</Button>)

    const button = screen.getByRole('button', { name: 'Saving' })
    expect(button).toHaveAttribute('aria-busy', 'true')
    const spinner = container.querySelector('[data-motion="busy"]')
    expect(spinner).not.toBeNull()
    expect(spinner).toHaveClass('animate-spin')
  })

  it('forwards native button attributes', () => {
    render(<Button type="submit">Submit</Button>)
    expect(screen.getByRole('button', { name: 'Submit' })).toHaveAttribute('type', 'submit')
  })
})
