import { type ArgumentMetadata, Injectable, type PipeTransform } from '@nestjs/common';
import { pointerIssues } from '@ngfw/schema';
import type { SchemaObject } from '@nestjs/swagger';
import { z } from 'zod';
import { problems } from './problem.js';
import { safeText } from './text.js';

/**
 * Request DTOs are Zod schemas (one definition for validation and OpenAPI — 00-CONTEXT rule 5). `ZodPipe` validates a
 * body/query and answers 400 problem+json with pointers; `openapi()` turns the same schema into an OpenAPI 3.1 schema
 * object for the swagger decorators.
 */
@Injectable()
export class ZodPipe<T extends z.ZodType> implements PipeTransform<unknown, z.output<T>> {
  constructor(private readonly schema: T) {}

  transform(value: unknown, meta: ArgumentMetadata): z.output<T> {
    const r = this.schema.safeParse(value ?? (meta.type === 'body' ? undefined : {}));
    if (!r.success) {
      throw problems.badRequest(
        `invalid ${meta.type === 'body' ? 'request body' : meta.type}`,
        pointerIssues(r.error),
      );
    }
    return r.data;
  }
}

export function openapi(schema: z.ZodType, io: 'input' | 'output' = 'input'): SchemaObject {
  const js = z.toJSONSchema(schema, {
    target: 'draft-2020-12',
    io,
    unrepresentable: 'any',
  }) as Record<string, unknown>;
  delete js['$schema'];
  return js as SchemaObject;
}

/** `$ref` to a component registered in buildOpenApi (the config document schemas from packages/schema). */
export function ref(name: string): SchemaObject {
  return { $ref: `#/components/schemas/${name}` } as SchemaObject;
}

/**
 * A path parameter as single-line safe text (TD-2 #4): 400 problem+json whose pointer names the parameter
 * (`/name`), instead of an empty pointer for a bare string.
 */
@Injectable()
export class SafeParamPipe implements PipeTransform<unknown, string> {
  private readonly schema: z.ZodType<Record<string, string>>;

  constructor(
    private readonly name: string,
    max = 128,
  ) {
    this.schema = z.object({ [name]: safeText(max).min(1) }) as z.ZodType<Record<string, string>>;
  }

  transform(value: unknown): string {
    const r = this.schema.safeParse({ [this.name]: value });
    if (!r.success)
      throw problems.badRequest(`invalid path parameter '${this.name}'`, pointerIssues(r.error));
    return r.data[this.name]!;
  }
}
