import 'reflect-metadata';
import { NestFactory } from '@nestjs/core';
import { FastifyAdapter, type NestFastifyApplication } from '@nestjs/platform-fastify';
import { DocumentBuilder, SwaggerModule, type OpenAPIObject } from '@nestjs/swagger';
import { AppModule } from './app.module.js';

export async function createApp(): Promise<NestFastifyApplication> {
  const app = await NestFactory.create<NestFastifyApplication>(AppModule, new FastifyAdapter(), {
    logger: ['error', 'warn', 'log'],
  });
  return app;
}

export function buildOpenApi(app: NestFastifyApplication): OpenAPIObject {
  const cfg = new DocumentBuilder()
    .setTitle('VRX API')
    .setDescription(
      'Management API of the VRX secure router. /config is transactional, /state is live read-only, /actions are imperative.',
    )
    .setVersion('0.1.0')
    .build();
  return SwaggerModule.createDocument(app, cfg);
}
