#!/bin/python3.11

import logging

from argparse import Namespace
from oci.config import from_file
from oci.auth.signers import InstancePrincipalsSecurityTokenSigner


class Config:
    def __init__(self, args: Namespace):
        self.log = logging.getLogger(f'{__name__}.Config')
        self.set_config(args)
        self.log.debug(f'Initalizing Config object: {self}')
        self.config, self.signer = self.set_signer(args)
        self.args = args

    # Set dependencies
    def set_config(self, args: Namespace):
        # Debug mode
        if args.debug:
            logging.basicConfig(level=logging.DEBUG)
            self.log.debug('Log Level set to DEBUG')
        else:
            logging.basicConfig(level=logging.INFO)

    def set_signer(self, args: Namespace) -> dict | None:
        if args.auth == None:
            self.log.debug(f'Creating signer from profile {args.profile}')
            config = from_file(profile_name=args.profile, file_location=args.config)
            self.log.debug(f'Config: {config}')
            return config, None
        elif args.auth == 'instance_principal':
            self.log.debug('Creating signer from instance principal')
            signer = InstancePrincipalsSecurityTokenSigner(log_requests=args.debug)
            return {}, signer