'use client';

import { cva, type VariantProps } from 'class-variance-authority';
import { cn } from 'cn';
import { Tabs as TabsPrimitive } from 'radix-ui';
import type * as React from 'react';

function Tabs({
  className,
  orientation = 'horizontal',
  ...props
}: React.ComponentProps<typeof TabsPrimitive.Root>) {
  return (
    <TabsPrimitive.Root
      data-slot="tabs"
      data-orientation={orientation}
      className={cn('group/tabs flex gap-2 data-horizontal:flex-col', className)}
      {...props}
    />
  );
}

const tabsListVariants = cva(
  'group/tabs-list inline-flex w-fit items-center justify-center rounded-lg p-[3px] text-muted-foreground group-data-horizontal/tabs:h-8 group-data-vertical/tabs:h-fit group-data-vertical/tabs:flex-col data-[variant=line]:rounded-none',
  {
    variants: {
      variant: {
        default: 'bg-muted',
        line: 'gap-1 bg-transparent',
        // Estilo "gradient button": sem fundo próprio, altura extra para o
        // pill selecionado (que tem anel de gradiente em volta) caber.
        gradient: 'gap-1.5 bg-transparent p-1 group-data-horizontal/tabs:h-10',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);

function TabsList({
  className,
  variant = 'default',
  ...props
}: React.ComponentProps<typeof TabsPrimitive.List> & VariantProps<typeof tabsListVariants>) {
  return (
    <TabsPrimitive.List
      data-slot="tabs-list"
      data-variant={variant}
      className={cn(tabsListVariants({ variant }), className)}
      {...props}
    />
  );
}

const tabsTriggerVariants = cva(
  // Base compartilhada: o AFTER é o sublinhado da variante "line"; o BEFORE
  // é a "tampa" da cor do fundo por cima do gradiente (pill selecionado).
  "relative inline-flex h-[calc(100%-1px)] flex-1 items-center justify-center gap-1.5 rounded-md border border-transparent px-1.5 py-0.5 text-sm font-medium whitespace-nowrap transition-all group-data-vertical/tabs:w-full group-data-vertical/tabs:justify-start focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-1 focus-visible:outline-ring disabled:pointer-events-none disabled:opacity-50 has-data-[icon=inline-end]:pr-1 has-data-[icon=inline-start]:pl-1 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default: [
          'text-foreground/60 hover:text-foreground dark:text-muted-foreground dark:hover:text-foreground',
          'group-data-[variant=default]/tabs-list:aria-selected:shadow-sm group-data-[variant=line]/tabs-list:aria-selected:shadow-none',
          'group-data-[variant=line]/tabs-list:bg-transparent group-data-[variant=line]/tabs-list:aria-selected:bg-transparent dark:group-data-[variant=line]/tabs-list:aria-selected:border-transparent dark:group-data-[variant=line]/tabs-list:aria-selected:bg-transparent',
          'aria-selected:bg-background aria-selected:text-foreground dark:aria-selected:border-input dark:aria-selected:bg-input/30 dark:aria-selected:text-foreground',
          'after:absolute after:bg-foreground after:opacity-0 after:transition-opacity group-data-horizontal/tabs:after:inset-x-0 group-data-horizontal/tabs:after:bottom-[-5px] group-data-horizontal/tabs:after:h-0.5 group-data-vertical/tabs:after:inset-y-0 group-data-vertical/tabs:after:-right-1 group-data-vertical/tabs:after:w-0.5 group-data-[variant=line]/tabs-list:aria-selected:after:opacity-100',
        ],
        // Estilo "gradient button" (tweakcn): não selecionado = só borda
        // (sem hover); selecionado = fundo gradiente cyan→blue por inteiro,
        // com o texto branco. Sem tampa/pseudo-elemento e sem transição de
        // hover — o estado selecionado é fixo.
        gradient: [
          'h-auto border-border bg-transparent px-4 py-2 leading-5 text-foreground/75',
          'focus-visible:border-cyan-300 focus-visible:ring-4 focus-visible:ring-cyan-200/70 dark:focus-visible:ring-cyan-800/70',
          'aria-selected:border-transparent aria-selected:bg-gradient-to-br aria-selected:from-cyan-500 aria-selected:to-blue-500 aria-selected:text-white aria-selected:shadow-sm',
        ],
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  },
);

function TabsTrigger({
  className,
  variant = 'default',
  ...props
}: React.ComponentProps<typeof TabsPrimitive.Trigger> & VariantProps<typeof tabsTriggerVariants>) {
  // Destaque do estado usa aria-selected (e não data-state): o trigger aqui
  // é envolvido por um TooltipTrigger asChild, e os DOIS primitivos do Radix
  // escrevem data-state no mesmo <button> — o do tooltip ("closed"/"open")
  // sobrescreve o "active" da aba. aria-selected é exclusivo do Radix Tabs.
  return (
    <TabsPrimitive.Trigger
      data-slot="tabs-trigger"
      data-variant={variant}
      className={cn(tabsTriggerVariants({ variant }), className)}
      {...props}
    />
  );
}

function TabsContent({ className, ...props }: React.ComponentProps<typeof TabsPrimitive.Content>) {
  return (
    <TabsPrimitive.Content
      data-slot="tabs-content"
      className={cn('flex-1 text-sm outline-none', className)}
      {...props}
    />
  );
}

export { Tabs, TabsContent, TabsList, TabsTrigger, tabsListVariants, tabsTriggerVariants };
