#!/bin/python3.11

import logging

from modules.config import Config
from oci import pagination
from oci.resource_search import ResourceSearchClient
from oci.resource_search.models import StructuredSearchDetails

class Search:

    search_types = [
    "instance",
    "volume",
    "vcn",
    "analyticsinstance",
    "apigateway",
    "bastion",
    "bootvolume",
    "certificateauthority",
    "instancepool",
    "clusterscluster",
    "clustersvirtualnode",
    "containerinstance",
    "datacatalog",
    "application",
    "disworkspace",
    "datascienceproject",
    "autonomousdatabase",
    "cloudexadatainfrastructure",
    "dbsystem",
    "devopsproject",
    "filesystem",
    "functionsfunction",
    "integrationinstance",
    "loadbalancer",
    "drg",
    "networkfirewall",
    "bucket",
    "stream",
    "vault",
    "vbsinstance"
    ]

    def __init__(self, config: Config):
        self.log = logging.getLogger(f'{__name__}.Search')
        self.log.info('Initializing Search Object')
        self.client = ResourceSearchClient(config.config, signer=config.signer)
        self.log.debug(f'Search client: {self.client}')

        # Create structured search with parameters
        comma = ", "
        query = (f"query {comma.join(self.search_types)} resources where"
                 f" (definedTags.namespace = '{config.args.tag}')")
        query += f" && compartmentId  = '{config.args.compartment}'" if \
            config.args.compartment else ""
        query += f" && compartmentId  != '{config.args.exclude}'" if \
            config.args.exclude else ""

        details = StructuredSearchDetails(query=query)
        response = pagination.list_call_get_all_results(self.client.search_resources, details)
        self.log.debug(f'Response Headers: {response.headers}\n\t'
                       f'Request ID: {response.request_id}\n\tRequest: '
                       f'{response.request}\n\tStatus: {response.status}')
        self.data = response.data.items
        self.log.debug(f'Initialized Search with data: {self.data}')
        self.log.info(f'Found {len(self.data)} Resources')